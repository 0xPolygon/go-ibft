package core

import (
	"bytes"
	"errors"
	"github.com/Trapesys/go-ibft/messages"
	"github.com/Trapesys/go-ibft/messages/proto"
)

type Logger interface {
	Info(msg string, args ...interface{})
	Debug(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

type Messages interface {
	AddMessage(message *proto.Message)
	NumMessages(view *proto.View, messageType proto.MessageType) int
	PruneByHeight(view *proto.View)
	PruneByRound(view *proto.View)

	GetPrePrepareMessage(view *proto.View) *messages.PrePrepareMessage
	GetPrepareMessages(view *proto.View) []*messages.PrepareMessage
	GetCommitMessages(view *proto.View) []*messages.CommitMessage
	GetRoundChangeMessages(view *proto.View) []*messages.RoundChangeMessage
}

var (
	errBuildProposal           = errors.New("failed to build proposal")
	errPrePrepareBlockMismatch = errors.New("(pre-prepare) proposal not matching locked block")
	errInvalidBlock            = errors.New("invalid block proposal")
	errPrepareHashMismatch     = errors.New("(prepare) block hash not matching accepted block")
	errQuorumNotReached        = errors.New("quorum on messages not reached")
	errInvalidCommittedSeal    = errors.New("invalid commit seal in commit message")
)

type QuorumFn func(num uint64) uint64

type view struct {
	height, round uint64
}

type stateName int

const (
	newRound stateName = iota
	prepare
	commit
	roundChange
	fin
)

type state struct {
	//	current view (block height, round)
	view proto.View

	//	block proposal for current round
	proposal []byte

	//	flags for different states
	roundStarted, locked bool

	name stateName
}

type IBFT struct {
	log Logger

	state state

	messages Messages

	backend Backend

	transport Transport

	quorumFn QuorumFn

	roundDone chan error
}

func NewIBFT(
	log Logger,
	backend Backend,
	transport Transport,
) *IBFT {
	return &IBFT{
		log:       log,
		backend:   backend,
		transport: transport,
		roundDone: make(chan error),
	}
}

func (i *IBFT) runSequence(h uint64) {
	//	TODO
}

func (i *IBFT) runRound(quit <-chan struct{}) {
	for {
		switch i.state.name {
		case newRound:
			if err := i.runNewRound(); err != nil {
				//	something wrong -> go to round change
				i.roundDone <- err
				//i.state.name = roundChange

				return
			}

		case prepare:
			i.runPrepare()
		case commit:
			i.runCommit()
		}

		//	TODO: check f+1 RC

		select {
		case <-quit:
			return
		default:
		}
	}
}

func (i *IBFT) runCommit() error {
	var (
		view           = &i.state.view
		height         = view.Height
		lockedProposal = i.state.proposal
		quorum         = i.quorumFn(i.backend.ValidatorCount(height))
	)

	//	get commit messages
	commitMessages := i.messages.GetCommitMessages(view)

	//	see if there is a quorum of them
	if len(commitMessages) < int(quorum) {
		return errQuorumNotReached
	}

	//	validate each (on error, go to round change)
	for _, msg := range commitMessages {
		if i.backend.IsValidCommittedSeal(lockedProposal, msg.CommittedSeal) {
			return errInvalidCommittedSeal
		}
	}

	//	block proposal finalized -> fin state
	i.state.name = fin

	return nil
}

func (i *IBFT) runPrepare() error {
	var (
		view          = &i.state.view
		acceptedBlock = i.state.proposal
		numValidators = i.backend.ValidatorCount(view.Height)
		quorum        = int(i.quorumFn(numValidators))
	)

	//	TODO: Q(P+C)

	prepareMessages := i.messages.GetPrepareMessages(view)
	if len(prepareMessages) < quorum {
		return errQuorumNotReached

	}

	//	TODO: if there is a quorum of valid prepare messages
	//		but some there are additional invalid prepare messages
	//		we can still go to commit state
	for _, msg := range i.messages.GetPrepareMessages(view) {
		if err := i.backend.VerifyProposalHash(acceptedBlock, msg.ProposalHash); err != nil {
			return errPrepareHashMismatch
		}
	}

	//	quorum on prepare message reached:
	//	lock on the accepted proposal and move to commit state
	i.state.name = commit
	i.state.locked = true

	commitMsg := i.backend.BuildCommitMessage(i.state.proposal)

	i.transport.Multicast(commitMsg)

	return nil
}

func (i *IBFT) runNewRound() error {
	var (
		height = i.state.view.Height
		round  = i.state.view.Round
		id     = []byte("my id") //	TODO (backend): id of this node
	)

	if i.backend.IsProposer(id, height, round) {
		return i.proposeBlock(height)
	}

	//	we are not the proposer, so we're checking on a PRE-PREPARE msg
	if i.messages.NumMessages(
		&i.state.view,
		proto.MessageType_PREPREPARE,
	) == 0 {
		//	no PRE-PREPARE message received (yet)
		return nil
	}

	//	TODO (messages): extract proposal from PRE-PREPARE message
	newProposal := []byte("new block")

	if err := i.acceptProposal(newProposal); err != nil {
		i.state.name = roundChange

		return err
	}

	//	TODO (backend): construct a PREPARE message and gossip
	prepare := &proto.Message{}
	i.transport.Multicast(prepare)

	return nil
}

func (i *IBFT) buildProposal(height uint64) ([]byte, error) {
	if i.state.locked {
		return i.state.proposal, nil
	}

	proposal, err := i.backend.BuildProposal(height)
	if err != nil {
		return nil, errBuildProposal
	}

	return proposal, nil

}

func (i *IBFT) proposeBlock(height uint64) error {
	proposal, err := i.buildProposal(height)
	if err != nil {
		i.state.name = roundChange

		return err
	}

	i.state.proposal = proposal
	i.state.name = prepare

	//	TODO (backend): construct a PREPARE message and gossip
	prepare := &proto.Message{}
	i.transport.Multicast(prepare)

	return nil
}

func (i *IBFT) validateProposal(newProposal []byte) error {
	//	In case I was previously locked on a block proposal,
	//	the new one must match the old
	if i.state.locked &&
		!bytes.Equal(i.state.proposal, newProposal) {
		//	proposed block does not match my locked block
		return errPrePrepareBlockMismatch
	}

	if !i.backend.IsValidBlock(newProposal) {
		return errInvalidBlock

	}

	return nil
}

func (i *IBFT) acceptProposal(proposal []byte) error {
	if err := i.validateProposal(proposal); err != nil {
		return err
	}

	//	accept newly proposed block and move to PREPARE state
	i.state.proposal = proposal
	i.state.name = prepare

	return nil
}
