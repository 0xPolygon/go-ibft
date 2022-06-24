package core

import (
	"bytes"
	"errors"
	"github.com/Trapesys/go-ibft/messages"
	"github.com/Trapesys/go-ibft/messages/proto"
	"math"
	"time"
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
	GetMostRoundChangeMessages() []*messages.RoundChangeMessage
}

var (
	errBuildProposal           = errors.New("failed to build proposal")
	errPrePrepareBlockMismatch = errors.New("(pre-prepare) proposal not matching locked block")
	errInvalidBlock            = errors.New("invalid block proposal")
	errPrepareHashMismatch     = errors.New("(prepare) block hash not matching accepted block")
	errQuorumNotReached        = errors.New("quorum on messages not reached")
	errInvalidCommittedSeal    = errors.New("invalid commit seal in commit message")
	errInsertBlock             = errors.New("failed to insert block")

	roundZeroTimeout = 10 * time.Second
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

	//	validated commit seals
	seals [][]byte

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

	roundTimer *time.Timer
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
	for {
		currentRound := i.state.view.Round
		quitCh := make(chan struct{})

		go i.runRound(quitCh)

		select {
		case <-i.roundTimeout(currentRound):
			close(quitCh)
			//	TODO: our round timer expired:
			//		increment round by 1
			//		multicast RC message

		case err := <-i.roundDone:
			i.stopRoundTimeout()
			if err == nil {
				//	block is finalized for this height, return
				return
			}
		}

		i.waitForNewRoundStart()
	}
}

func (i *IBFT) waitForNewRoundStart() {
	for {
		msgs := i.messages.GetMostRoundChangeMessages()
		//	wait for quorum of rc messages
		if len(msgs) < int(i.quorumFn(i.backend.ValidatorCount(
			i.state.view.Height))) {
			continue
		}

		newRound := msgs[0].Round
		if i.state.view.Round < newRound ||
			(i.state.view.Round == newRound && !i.state.roundStarted) {
			i.state.view.Round = newRound
			break
		}

	}
}

func (i *IBFT) roundTimeout(round uint64) <-chan time.Time {
	var (
		duration    = int(roundZeroTimeout)
		roundFactor = int(math.Pow(float64(2), float64(round)))
	)

	i.roundTimer = time.NewTimer(time.Duration(duration * roundFactor))

	return i.roundTimer.C
}

func (i *IBFT) stopRoundTimeout() {
	i.roundTimer.Stop()
}

func (i *IBFT) runRound(quit <-chan struct{}) {
	i.state.name = newRound
	i.state.roundStarted = true

	for {
		err := i.runState()

		if errors.Is(err, errPrePrepareBlockMismatch) ||
			errors.Is(err, errInvalidCommittedSeal) {
			//	consensus err -> go to round change
			i.roundDone <- err

			return
		}

		if errors.Is(err, errInsertBlock) {
			//	TODO: ??? (not a consensus error)
			return
		}

		msgs := i.messages.GetMostRoundChangeMessages()
		if len(msgs) > 0 {
			suggestedRound := msgs[0].Round
			if i.state.view.Round < suggestedRound &&
				len(msgs) > 1000+1 {
				//	TODO: 1000 + 1 -> f(n) + 1
				//		move to new round
				i.state.view.Round = suggestedRound
				i.state.proposal = nil
				i.state.roundStarted = false

				i.transport.Multicast(
					i.backend.BuildRoundChangeMessage(
						i.state.view.Height,
						suggestedRound,
					))

				i.roundDone <- errors.New("higher round change received")

				return
			}
		}

		select {
		case <-quit:
			return
		default:
		}
	}
}

func (i *IBFT) runState() error {
	switch i.state.name {
	case newRound:
		return i.runNewRound()
	case prepare:
		return i.runPrepare()
	case commit:
		return i.runCommit()
	case fin:
		return i.runFin()
	default:
		//	wat
		return nil
	}
}

func (i *IBFT) runFin() error {
	if err := i.backend.InsertBlock(
		i.state.proposal,
		i.state.seals,
	); err != nil {
		return errInsertBlock
	}

	return nil
}

func (i *IBFT) commitMessages(view *proto.View) []*messages.CommitMessage {
	var valid []*messages.CommitMessage

	for _, msg := range i.messages.GetCommitMessages(view) {
		//	check hash
		if err := i.backend.VerifyProposalHash(
			i.state.proposal,
			msg.ProposalHash,
		); err != nil {
			continue
		}

		//	check commit seal
		if !i.backend.IsValidCommittedSeal(
			i.state.proposal,
			msg.CommittedSeal,
		) {
			continue
		}

		valid = append(valid, msg)
	}

	return valid
}

func (i *IBFT) runCommit() error {
	var (
		view   = &i.state.view
		quorum = int(i.quorumFn(i.backend.ValidatorCount(view.Height)))
	)

	//	get commit messages
	commitMessages := i.commitMessages(view)
	if len(commitMessages) < quorum {
		return errQuorumNotReached
	}

	//	add seals
	for _, msg := range commitMessages {
		//	TODO: these need to be pruned before each new round
		i.state.seals = append(i.state.seals, msg.CommittedSeal)
	}

	//	block proposal finalized -> fin state
	i.state.name = fin

	return nil
}

func (i *IBFT) prepareMessages(view *proto.View) []*messages.PrepareMessage {
	var valid []*messages.PrepareMessage

	for _, msg := range i.messages.GetPrepareMessages(view) {
		if err := i.backend.VerifyProposalHash(
			i.state.proposal,
			msg.ProposalHash,
		); err != nil {
			continue
		}

		valid = append(valid, msg)
	}

	return valid
}

func (i *IBFT) runPrepare() error {
	var (
		view          = &i.state.view
		numValidators = i.backend.ValidatorCount(view.Height)
		quorum        = int(i.quorumFn(numValidators))
	)

	//	TODO: Q(P+C)
	if len(i.prepareMessages(view)) < quorum {
		return errQuorumNotReached
	}

	i.state.name = commit
	i.state.locked = true

	i.transport.Multicast(
		i.backend.BuildCommitMessage(i.state.proposal),
	)

	return nil
}

func (i *IBFT) runNewRound() error {
	var (
		view   = &i.state.view
		height = view.Height
		round  = view.Round
	)

	if i.backend.IsProposer(i.backend.ID(), height, round) {
		return i.proposeBlock(height)
	}

	//	we are not the proposer, so we're checking on a PRE-PREPARE msg
	preprepareMsg := i.messages.GetPrePrepareMessage(view)
	if preprepareMsg == nil {
		//	no PRE-PREPARE message received yet
		return nil
	}

	newProposal := preprepareMsg.Proposal
	if err := i.acceptProposal(newProposal); err != nil {
		i.state.name = roundChange
		i.state.roundStarted = false

		return err
	}

	i.transport.Multicast(
		i.backend.BuildPrepareMessage(newProposal),
	)

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

	i.transport.Multicast(
		i.backend.BuildPrepareMessage(proposal),
	)

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
