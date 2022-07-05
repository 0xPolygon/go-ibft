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
	// Messages modifiers //

	AddMessage(message *proto.Message)
	PruneByHeight(view *proto.View)

	// Messages fetchers //

	GetValidMessages(
		view *proto.View,
		messageType proto.MessageType,
		isValid func(*proto.Message) bool,
	) []*proto.Message
	GetMostRoundChangeMessages(minRound, height uint64) []*proto.Message

	// Messages subscription handlers //

	Subscribe(details messages.SubscriptionDetails) *messages.SubscribeResult
	Unsubscribe(id messages.SubscriptionID)
}

var (
	errBuildProposal        = errors.New("failed to build proposal")
	errInvalidBlockProposal = errors.New("invalid block proposal")
	errInvalidBlockProposed = errors.New("invalid block proposed")
	errInsertBlock          = errors.New("failed to insert block")
	errQuitReceived         = errors.New("quit signal received")

	roundZeroTimeout = 10 * time.Second
)

// IBFT represents a single instance of the IBFT state machine
type IBFT struct {
	// log is the logger instance
	log Logger

	// state is the current IBFT node state
	state *state

	// messages is the message storage layer
	messages Messages

	// backend is the reference to the
	// Backend implementation
	backend Backend

	// transport is the reference to the
	// Transport implementation
	transport Transport

	// roundDone is the channel used for signalizing
	// consensus finalization upon a certain sequence
	roundDone chan struct{}

	// roundChange is the channel used for signalizing
	// round changing events
	roundChange chan uint64

	// wg is a simple barrier used for synchronizing
	// state modification routines
	wg sync.WaitGroup

	// roundTimer is the main timer for a single round
	roundTimer *time.Timer
}

// NewIBFT creates a new instance of the IBFT consensus protocol
func NewIBFT(
	log Logger,
	backend Backend,
	transport Transport,
) *IBFT {
	return &IBFT{
		log:       log,
		backend:   backend,
		transport: transport,
		messages:  messages.NewMessages(),
		state: &state{
			view: &proto.View{
				Height: 0,
				Round:  0,
			},
			proposal:     nil,
			seals:        make([][]byte, 0),
			roundStarted: false,
			locked:       false,
			name:         newRound,
		},
		roundDone:   make(chan struct{}),
		roundChange: make(chan uint64),
	}
}

// startRoundTimer starts the exponential round timer, based on the
// passed in round number
func (i *IBFT) startRoundTimer(round uint64, quit <-chan struct{}) {
	var (
		duration     = int(roundZeroTimeout)
		roundFactor  = int(math.Pow(float64(2), float64(round)))
		roundTimeout = time.Duration(duration * roundFactor)
	)

	//	Create a new timer instance
	timer := time.NewTimer(roundTimeout)

	select {
	case <-quit:
		// Stop signal received, stop the timer
		timer.Stop()
	case <-timer.C:
		// Timer expired, alert the round change channel to move
		// to the next round
		i.roundChange <- round + 1
	}

	return
}

// watchForRoundHop checks if there are F+1 Round Change messages
// at any point in time to trigger a round hop to the highest round
// which has F+1 Round Change messages
func (i *IBFT) watchForRoundHop(quit <-chan struct{}) {
	view := i.state.getView()

	for {
		// Get the messages from the message queue
		rcMessages := i.messages.
			GetMostRoundChangeMessages(
				view.Round,
				view.Height,
			)

		//	Signal round change if enough round change messages were received
		if len(rcMessages) >= int(i.backend.MaximumFaultyNodes()) {
			// The round in the Round Change messages should be the highest
			// round for which there are F+1 RC messages
			newRound := rcMessages[0].View.Round
			i.roundChange <- newRound

			return
		}

		select {
		case <-quit:
			// Quit signal received, teardown the worker
			return
		default:
		}
	}
}

// runSequence runs the consensus cycle for the specified block height
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

		// Start the round timer worker
		go i.startRoundTimer(currentRound, quitCh)

		// Start the state machine worker
		go i.runRound(quitCh)

		// Start the round hop worker
		go i.watchForRoundHop(quitCh)

		select {
		case newRound := <-i.roundChange:
			// Round Change request received.
			// Stop all running worker threads
			close(quitCh)
			i.wg.Wait()

			//	Move to the new round
			i.moveToNewRoundWithRC(newRound, i.state.getHeight())
			i.state.setLocked(false)
		case <-i.roundDone:
			// The consensus cycle for the block height is finished.
			// Stop all running worker threads
			close(quitCh)

			return
		}

		// Wait to reach quorum on what the next round
		// should be before starting the cycle again
		i.runRoundChange()
	}
}

// runRound runs the state machine loop for the current round
func (i *IBFT) runRound(quit <-chan struct{}) {
	// Register this worker thread with the barrier
	i.wg.Add(1)
	defer i.wg.Done()

	//	TODO: is if needed  (for tests)?
	if !i.state.roundStarted {
		// Round is not yet started, kick the round off
		i.state.name = newRound
		i.state.roundStarted = true
	}

	// Check if any block needs to be proposed
	if i.backend.IsProposer(
		i.backend.ID(),
		i.state.getHeight(),
		i.state.getRound(),
	) {
		// The current node is the proposer, submit the proposal
		// to other nodes
		if err := i.proposeBlock(i.state.getHeight()); err != nil {
			// Proposal is unable to be submitted, move to the round change state
			i.roundChange <- i.state.getRound() + 1

			return
		}
	}

	for {
		var err error

		switch i.state.name {
		case newRound:
			err = i.runNewRound(quit)
		case prepare:
			err = i.runPrepare(quit)
		case commit:
			err = i.runCommit(quit)
		case fin:
			if err = i.runFin(); err == nil {
				//	Block inserted without any errors,
				// sequence is complete
				i.roundDone <- struct{}{}

				return
			}
		}

		if err != nil {
			// There was a critical consensus error (or timeout) during
			// state execution, move to the round change state
			i.roundChange <- i.state.getRound() + 1

			return
		}
	}
}

// runNewRound runs the New Round IBFT state
func (i *IBFT) runNewRound(quit <-chan struct{}) error {
	var (
		// Grab the current view
		view = i.state.getView()

		// Subscribe for PREPREPARE messages
		messageType = proto.MessageType_PREPREPARE
		sub         = i.messages.Subscribe(
			messages.SubscriptionDetails{
				MessageType: messageType,
				View:        view,
				NumMessages: 1,
			},
		)
	)

	// The subscription is not needed anymore after
	// this state is done executing
	defer i.messages.Unsubscribe(sub.GetID())

	for {
		select {
		case <-quit:
			// Stop signal received, exit
			return errQuitReceived
		case <-sub.GetCh():
			// Subscription conditions have been met,
			// grab the proposal messages
			var proposal []byte

			isValidFn := func(message *proto.Message) bool {
				// Verify that the message is indeed from the proposer for this view
				return i.backend.IsProposer(message.From, view.Height, view.Round)
			}

			preprepareMessages := i.messages.GetValidMessages(view, messageType, isValidFn)
			for _, message := range preprepareMessages {
				// Make sure that the proposer's PREPREPARE proposal is valid
				proposal = messages.ExtractProposal(message)

				if err := i.validateProposal(proposal); err != nil {
					return err
				}

				break
			}

			// Accept the proposal since it's valid
			i.acceptProposal(proposal)

			// Multicast the PREPARE message
			i.transport.Multicast(
				i.backend.BuildPrepareMessage(proposal, i.state.getView()),
			)

			// Move to the prepare state
			i.state.name = prepare

			return nil
		}
	}
}

// runPrepare runs the Prepare IBFT state
func (i *IBFT) runPrepare(quit <-chan struct{}) error {
	var (
		// Grab the current view
		view = i.state.getView()

		// Grab quorum information
		quorum = i.backend.Quorum(view.Height)

		// Subscribe to PREPARE messages
		messageType = proto.MessageType_PREPARE
		sub         = i.messages.Subscribe(
			messages.SubscriptionDetails{
				MessageType: messageType,
				View:        view,
				NumMessages: int(quorum),
			},
		)
	)

	// The subscription is not needed anymore after
	// this state is done executing
	defer i.messages.Unsubscribe(sub.GetID())

	for {
		select {
		case <-quit:
			// Stop signal received, exit
			return errQuitReceived
		case <-sub.GetCh():
			// Subscription conditions have been met,
			// grab the prepare messages
			isValidFn := func(message *proto.Message) bool {
				// Verify that the proposal hash is valid
				return i.backend.IsValidProposalHash(
					i.state.proposal,
					messages.ExtractPrepareHash(message),
				)
			}

			prepareMessages := i.messages.GetValidMessages(view, messageType, isValidFn)

			if uint64(len(prepareMessages)) < quorum {
				//	quorum not reached, keep polling
				continue
			}

			// Multicast the COMMIT message
			i.transport.Multicast(
				i.backend.BuildCommitMessage(i.state.proposal, view),
			)

			//	Move to the commit state
			i.state.name = commit

			return nil
		}
	}
}

// runCommit runs the Commit IBFT state
func (i *IBFT) runCommit(quit <-chan struct{}) error {
	var (
		// Grab the current view
		view = i.state.getView()

		// Grab quorum information
		quorum = i.backend.Quorum(view.Height)

		// Subscribe to COMMIT messages
		messageType = proto.MessageType_COMMIT
		sub         = i.messages.Subscribe(
			messages.SubscriptionDetails{
				MessageType: messageType,
				View:        view,
				NumMessages: int(quorum),
			},
		)
	)

	// The subscription is not needed anymore after
	// this state is done executing
	defer i.messages.Unsubscribe(sub.GetID())

	for {
		select {
		case <-quit:
			// Stop signal received, exit
			return errQuitReceived
		case <-sub.GetCh():
			// Subscription conditions have been met,
			// grab the commit messages
			isValidFn := func(message *proto.Message) bool {
				//	Verify that the proposal hash is valid
				proposalHash := messages.ExtractCommitHash(message)

				if !i.backend.IsValidProposalHash(
					i.state.proposal,
					messages.ExtractCommitHash(message),
				) {
					return false
				}

				//	Verify that the committed seal is valid
				committedSeal := messages.ExtractCommittedSeal(message)
				return i.backend.IsValidCommittedSeal(proposalHash, committedSeal)
			}

			commitMessages := i.messages.GetValidMessages(view, messageType, isValidFn)

			if uint64(len(commitMessages)) < quorum {
				//	quorum not reached, keep polling
				continue
			}

			// Set the committed seals
			i.state.seals = messages.ExtractCommittedSeals(commitMessages)

			//	Move to the fin state
			i.state.name = fin

			return nil
		}
	}
}

// runFin runs the fin state (block insertion)
func (i *IBFT) runFin() error {
	// Insert the block to the node's underlying
	// blockchain layer
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

// runRoundChange runs the Round Change IBFT state
func (i *IBFT) runRoundChange() {
	var (
		// Grab the current view
		view = i.state.getView()

		// Grab quorum information
		quorum = i.backend.Quorum(view.Height)

		// Subscribe to ROUND CHANGE messages
		sub = i.messages.Subscribe(
			messages.SubscriptionDetails{
				MessageType: proto.MessageType_ROUND_CHANGE,
				View:        view,
				NumMessages: int(quorum),
			},
		)
	)

	// The subscription is not needed anymore after
	// this state is done executing
	defer i.messages.Unsubscribe(sub.GetID())

	// Wait until a quorum of Round Change messages
	// has been received in order to start the new round
	<-sub.GetCh()
}

// moveToNewRound moves the state to the new round
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
func (i *IBFT) AddMessage(message *proto.Message) {
	// Make sure the message is present
	if message == nil {
		return
	}

	// Check if the message should even be considered
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
	// the current state height, or greater
	if i.state.getHeight() >= message.View.Height {
		return false
	}

	// Make sure the message round is >= the current state round
	return message.View.Round >= i.state.getRound()
}
