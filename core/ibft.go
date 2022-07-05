package core

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/Trapesys/go-ibft/messages"
	"github.com/Trapesys/go-ibft/messages/proto"
	"math"
	"sync"
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

	GetMessages(view *proto.View, messageType proto.MessageType) []*proto.Message
	GetProposal(view *proto.View) []byte
	GetCommittedSeals(view *proto.View) [][]byte
	GetMostRoundChangeMessages(minRound, height uint64) []*proto.Message

	Subscribe(details messages.SubscriptionDetails) *messages.SubscribeResult
	Unsubscribe(id messages.SubscriptionID)
}

var (
	errBuildProposal        = errors.New("failed to build proposal")
	errInvalidBlockProposal = errors.New("invalid block proposal")
	errInvalidBlockProposed = errors.New("invalid block proposed")
	errInvalidProposer      = errors.New("invalid block proposer")
	errInvalidCommittedSeal = errors.New("invalid commit seal in commit message")
	errInsertBlock          = errors.New("failed to insert block")
	errViewMismatch         = errors.New("invalid message view")
	errHashMismatch         = errors.New("data hash mismatch")

	roundZeroTimeout = 10 * time.Second
)

type IBFT struct {
	log Logger

	state *state

	messages Messages

	backend Backend

	transport Transport

	roundDone   chan error
	roundChange chan uint64

	wg sync.WaitGroup

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
		state: &state{
			view: &proto.View{
				Height: 0,
				Round:  0,
			},
			proposal:     nil,
			seals:        make([][]byte, 0),
			roundStarted: false,
			locked:       false,
			name:         0,
		},
		roundDone: make(chan error),
		//	TODO: create roundChange
	}
}

func (i *IBFT) startRoundTimer(round uint64, quit <-chan struct{}) {
	var (
		duration     = int(roundZeroTimeout)
		roundFactor  = int(math.Pow(float64(2), float64(round)))
		roundTimeout = time.Duration(duration * roundFactor)
	)

	//	timer for this round
	timer := time.NewTimer(roundTimeout)

	select {
	case <-quit:
		timer.Stop()
	case <-timer.C:
		i.roundChange <- round + 1
	}

	return
}

func (i *IBFT) watchForRoundHop(quit <-chan struct{}) {
	view := i.state.getView()

	for {
		rcMessages := i.messages.
			GetMostRoundChangeMessages(
				view.Round,
				view.Height)

		//	signal round change if enough round change messages were received
		if len(rcMessages) >= int(i.backend.AllowedFaulty()) {
			newRound := rcMessages[0].View.Round
			i.roundChange <- newRound

			return
		}

		//	return if this goroutine is cancelled
		select {
		case <-quit:
			return
		default:
		}
	}
}

func (i *IBFT) runRoundChange() {
	var (
		view   = i.state.getView()
		quorum = i.backend.Quorum(view.Height)
	)

	sub := i.messages.Subscribe(
		messages.SubscriptionDetails{
			MessageType: proto.MessageType_ROUND_CHANGE,
			View:        view,
			NumMessages: int(quorum),
		})

	defer i.messages.Unsubscribe(sub.GetID())

	//	wait until we have received a quorum
	//	od RC messages in order to start the new round
	<-sub.GetCh()

}

func (i *IBFT) runSequence(h uint64) {
	// TODO do state clear here
	// Set the starting state data
	i.state.setView(&proto.View{
		Height: h,
		Round:  0,
	})

	for {
		currentRound := i.state.getRound()
		quitCh := make(chan struct{})

		go i.startRoundTimer(currentRound, quitCh)
		go i.runRound(quitCh)
		go i.watchForRoundHop(quitCh)

		select {
		case newRound := <-i.roundChange:
			//	stop all running goroutines
			close(quitCh)
			i.wg.Wait()

			//	move to new round
			i.moveToNewRoundWithRC(newRound, i.state.getHeight())
			i.state.setLocked(false)
		case _ = <-i.roundDone:
			close(quitCh)
			//	TODO: check error

		}

		i.runRoundChange()
	}
}

func (i *IBFT) runRound(quit <-chan struct{}) {
	i.wg.Add(1)
	defer i.wg.Done()

	//	TODO: is if needed  (for tests)?
	if !i.state.roundStarted {
		i.state.name = newRound
		i.state.roundStarted = true
	}

	if i.backend.IsProposer(
		i.backend.ID(),
		i.state.getHeight(),
		i.state.getRound(),
	) {
		if err := i.proposeBlock(i.state.getHeight()); err != nil {
			i.roundChange <- i.state.getRound() + 1

			return
		}
	}

	//	TODO: state loop
	for {
		var err error

		switch i.state.name {
		case newRound:
			if err = i.runNewRound(quit); err != nil {
				i.roundChange <- i.state.getRound() + 1

				return
			}
		case prepare:
			//	TODO: cannot possibly error here
			_ = i.runPrepare(quit)
		case commit:
			//	TODO: cannot possibly error here
			_ = i.runCommit(quit)
		case fin:
			if err = i.runFin(); err != nil {
				i.roundChange <- i.state.getRound() + 1

				return
			}
		}

	}
}

func (i *IBFT) runNewRound(quit <-chan struct{}) error {
	view := i.state.getView()
	sub := i.messages.Subscribe(
		messages.SubscriptionDetails{
			MessageType: proto.MessageType_PREPREPARE,
			View:        view,
			NumMessages: 1,
		})

	defer i.messages.Unsubscribe(sub.GetID())

	for {
		select {
		case <-quit:
			return errors.New("round timeout expired")
		case <-sub.GetCh():
			var proposal []byte

			msgs := i.messages.GetMessages(view, proto.MessageType_PREPREPARE)
			for _, msg := range msgs {
				if !i.backend.IsProposer(msg.From, view.Height, view.Round) {
					continue
				}

				proposal = msg.Payload.(*proto.Message_PreprepareData).PreprepareData.Proposal

				if err := i.validateProposal(proposal); err != nil {
					return err
				}

				break
			}

			i.acceptProposal(proposal)

			//	multicast PREPARE message
			i.transport.Multicast(
				i.backend.BuildPrepareMessage(proposal, i.state.getView()),
			)

			//	set state to PREPARE and return
			i.state.name = prepare

			return nil
		}
	}
}

func (i *IBFT) runPrepare(quit <-chan struct{}) error {
	var (
		view   = i.state.getView()
		quorum = i.backend.Quorum(view.Height)
	)

	sub := i.messages.Subscribe(
		messages.SubscriptionDetails{
			MessageType: proto.MessageType_PREPARE,
			View:        view,
			NumMessages: int(quorum),
		})

	defer i.messages.Unsubscribe(sub.GetID())

	for {
		select {
		case <-quit:
			return errors.New("round timeout expired")
		case <-sub.GetCh():
			validCount := uint64(0)

			//	get messages
			msgs := i.messages.GetMessages(view, proto.MessageType_PREPARE)
			for _, msg := range msgs {
				proposalHash := msg.Payload.(*proto.Message_PrepareData).PrepareData.ProposalHash
				if err := i.backend.VerifyProposalHash(i.state.proposal, proposalHash); err != nil {
					continue
				}

				validCount++
				if validCount >= quorum {
					break
				}
			}

			if validCount < quorum {
				//	quorum not reached, keep polling
				continue
			}

			//	multicast COMMIT message
			i.transport.Multicast(
				i.backend.BuildCommitMessage(i.state.proposal, view),
			)

			//	move to commit state and return
			i.state.name = commit

			return nil
		}
	}
}

func (i *IBFT) runCommit(quit <-chan struct{}) error {
	var (
		view   = i.state.getView()
		quorum = i.backend.Quorum(view.Height)
	)

	sub := i.messages.Subscribe(
		messages.SubscriptionDetails{
			MessageType: proto.MessageType_COMMIT,
			View:        view,
			NumMessages: int(quorum),
		})

	defer i.messages.Unsubscribe(sub.GetID())

	for {
		select {
		case <-quit:
			return errors.New("round timeout expired")
		case <-sub.GetCh():
			validCount := uint64(0)

			//	get messages
			msgs := i.messages.GetMessages(view, proto.MessageType_COMMIT)
			for _, msg := range msgs {

				//	verify hash
				proposalHash := msg.Payload.(*proto.Message_CommitData).CommitData.ProposalHash
				if err := i.backend.VerifyProposalHash(i.state.proposal, proposalHash); err != nil {
					continue
				}

				//	verify committed seal
				committedSeal := msg.Payload.(*proto.Message_CommitData).CommitData.CommittedSeal
				if !i.backend.IsValidCommittedSeal(proposalHash, committedSeal) {
					continue
				}

				validCount++
				if validCount >= quorum {
					break
				}
			}

			if validCount < quorum {
				//	quorum not reached, keep polling
				continue
			}

			//	move to FIN state and return
			i.state.name = fin

			return nil
		}
	}
}

// runFin runs the fin state (block insertion)
func (i *IBFT) runFin() error {
	if err := i.backend.InsertBlock(
		i.state.getProposal(),
		i.state.getCommittedSeals(),
	); err != nil {
		return errInsertBlock
	}

	// Remove stale messages
	view := i.state.getView()
	i.messages.PruneByHeight(view)

	return nil
}

// moveToNewRound moves the state to the new round change
func (i *IBFT) moveToNewRound(round, height uint64) {
	i.state.setView(&proto.View{
		Height: height,
		Round:  round,
	})

	i.state.setRoundStarted(false)
	i.state.setProposal(nil)
	i.state.setStateName(roundChange)

	i.log.Info("moved to new round", round, height)
}

// moveToNewRoundWithRC moves the state to the new round change
// and multicasts an appropriate Round Change message
func (i *IBFT) moveToNewRoundWithRC(round, height uint64) {
	i.moveToNewRound(round, height)

	i.transport.Multicast(
		i.backend.BuildRoundChangeMessage(
			height,
			round,
		),
	)

	i.log.Info(fmt.Sprintf("multicasted RC round=%d height=%d", round, height))
}

// buildProposal builds a new proposal
func (i *IBFT) buildProposal(height uint64) ([]byte, error) {
	if i.state.isLocked() {
		return i.state.getProposal(), nil
	}

	proposal, err := i.backend.BuildProposal(height)
	if err != nil {
		return nil, errBuildProposal
	}

	return proposal, nil
}

// proposeBlock proposes a block to other peers through multicast
func (i *IBFT) proposeBlock(height uint64) error {
	proposal, err := i.buildProposal(height)
	if err != nil {
		return err
	}

	i.acceptProposal(proposal)
	i.log.Info("proposal accepted")

	i.transport.Multicast(
		i.backend.BuildPrePrepareMessage(proposal, i.state.getView()),
	)

	i.log.Info("proposal multicasted")

	i.transport.Multicast(
		i.backend.BuildPrepareMessage(proposal, i.state.getView()),
	)

	i.log.Info("prepare multicasted")

	return nil
}

// validateProposal validates that the proposal is valid
func (i *IBFT) validateProposal(newProposal []byte) error {
	//	In case I was previously locked on a block proposal,
	//	the new one must match the old
	if i.state.isLocked() &&
		!bytes.Equal(i.state.getProposal(), newProposal) {
		//	proposed block does not match my locked block
		return errInvalidBlockProposed
	}

	if !i.backend.IsValidBlock(newProposal) {
		return errInvalidBlockProposal
	}

	return nil
}

// acceptProposal accepts the proposal and moves the state
func (i *IBFT) acceptProposal(proposal []byte) {
	//	accept newly proposed block and move to PREPARE state
	i.state.setProposal(proposal)
	i.state.setStateName(prepare)
}

// AddMessage adds a new message to the IBFT message system
// TODO should this return an error?
func (i *IBFT) AddMessage(message *proto.Message) {
	// Make sure the message is present
	if message == nil {
		return
	}

	if i.isAcceptableMessage(message) {
		i.messages.AddMessage(message)
	}
}

// isAcceptableMessage checks if the message can even be accepted
func (i *IBFT) isAcceptableMessage(message *proto.Message) bool {
	//	Make sure the message sender is ok
	if !i.backend.IsValidSender(message) {
		return false
	}

	// Invalid messages are discarded
	// TODO move to specific format checker method
	if message.View == nil {
		return false
	}

	// Make sure the message is in accordance with
	// the current state height
	if i.state.getHeight() >= message.View.Height {
		return false
	}

	// Make sure the message round is >= the current state round
	return message.View.Round >= i.state.getRound()
}
