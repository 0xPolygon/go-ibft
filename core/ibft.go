package core

import (
	"bytes"
	"context"
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
	PruneByHeight(height uint64)

	// Messages fetchers //

	GetValidMessages(
		view *proto.View,
		messageType proto.MessageType,
		isValid func(*proto.Message) bool,
	) []*proto.Message
	GetMostRoundChangeMessages(minRound, height uint64) []*proto.Message

	// Messages subscription handlers //

	Subscribe(details messages.SubscriptionDetails) *messages.Subscription
	Unsubscribe(id messages.SubscriptionID)
}

var (
	errBuildProposal        = errors.New("failed to build proposal")
	errInvalidBlockProposal = errors.New("invalid block proposal")
	errInvalidBlockProposed = errors.New("invalid block proposed")
	errInsertBlock          = errors.New("failed to insert block")
	errTimeoutExpired       = errors.New("round timeout expired")
	errQuorumNotReached     = errors.New("quorum messages not reached")

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

	// roundTimer is the channel used for signalizing
	// round changing events
	roundTimer chan uint64

	newProposal chan newProposalEvent

	roundCertificate chan uint64

	//	User configured additional timeout for each round of consensus
	additionalTimeout time.Duration

	// wg is a simple barrier used for synchronizing
	// state modification routines
	wg sync.WaitGroup
}

// NewIBFT creates a new instance of the IBFT consensus protocol
func NewIBFT(
	log Logger,
	backend Backend,
	transport Transport,
) *IBFT {
	return &IBFT{
		log:         log,
		backend:     backend,
		transport:   transport,
		messages:    messages.NewMessages(),
		roundDone:   make(chan struct{}),
		roundTimer:  make(chan uint64),
		newProposal: make(chan newProposalEvent),
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
	}
}

// startRoundTimer starts the exponential round timer, based on the
// passed in round number
func (i *IBFT) startRoundTimer(ctx context.Context, round uint64, baseTimeout time.Duration) {
	defer i.wg.Done()

	var (
		duration     = int(baseTimeout)
		roundFactor  = int(math.Pow(float64(2), float64(round)))
		roundTimeout = time.Duration(duration * roundFactor)
	)

	//	Create a new timer instance
	timer := time.NewTimer(roundTimeout + i.additionalTimeout)

	select {
	case <-ctx.Done():
		// Stop signal received, stop the timer
		timer.Stop()
	case <-timer.C:
		// Timer expired, alert the round change channel to move
		// to the next round
		i.log.Info("round timer expired, alerting of change")
		i.signalRoundChange(ctx, round+1)
	}
}

// watchForRoundHop checks if there are F+1 Round Change messages
// at any point in time to trigger a round hop to the highest round
// which has F+1 Round Change messages
func (i *IBFT) watchForRoundHop(ctx context.Context) {
	defer i.wg.Done()

	view := i.state.getView()

	for {
		select {
		case <-ctx.Done():
			// Quit signal received, teardown the worker
			return
		default:
			//	quorum of round change messages reached, teardown the worker
			if i.doRoundHop(ctx, view) {
				i.log.Debug("quitting round after signal", "round", view.Round)

				return
			}
		}
	}
}

//	doRoundHop signals a round change if a quorum of round change messages was received
func (i *IBFT) doRoundHop(ctx context.Context, view *proto.View) bool {
	var (
		currentRound = view.Round
		height       = view.Height
		hopQuorum    = i.backend.MaximumFaultyNodes() + 1
	)

	// Get round change messages with the highest count from a future round
	rcMessages := i.messages.GetMostRoundChangeMessages(currentRound+1, height)

	//	Signal round hop if quorum is reached
	if len(rcMessages) >= int(hopQuorum) {
		// The round in the Round Change messages should be the highest
		// round for which there are F+1 RC messages
		newRound := rcMessages[0].View.Round

		i.log.Info(
			fmt.Sprintf(
				"round hop detected, alerting round change, current=%d new=%d",
				currentRound,
				newRound,
			),
		)

		i.signalRoundChange(ctx, newRound)

		return true
	}

	return false
}

//	signalRoundChange notifies the sequence routine (RunSequence) that it
//	should move to a new round. The quit channel is used to abort this call
//	if another routine has already signaled a round change request.
func (i *IBFT) signalRoundChange(ctx context.Context, round uint64) {
	select {
	case i.roundTimer <- round:
		i.log.Debug("signal round change", "new round", round)
	case <-ctx.Done():
	}
}

type newProposalEvent struct {
	proposal []byte
	round    uint64
}

func (i *IBFT) signalNewProposal(ctx context.Context, event newProposalEvent) {
	select {
	case i.newProposal <- event:
		i.log.Debug("signal new proposal")
	case <-ctx.Done():
	}
}

func (i *IBFT) CancelSequence() {
	//	pre-emptively send a signal to roundDone
	//	so the thread can finish early
	i.roundDone <- struct{}{}
}

// watchForFutureProposal listens for new proposal messages
// that are intended for higher rounds
func (i *IBFT) watchForFutureProposal(ctx context.Context) {
	var (
		view      = i.state.getView()
		height    = view.Height
		nextRound = view.Round + 1

		quorum = int(i.backend.Quorum(height))

		sub = i.messages.Subscribe(
			messages.SubscriptionDetails{
				MessageType: proto.MessageType_PREPREPARE,
				View: &proto.View{
					Height: height,
					Round:  nextRound,
				},
				NumMessages: 1,
				HasMinRound: true,
			},
		)
	)

	defer func() {
		i.messages.Unsubscribe(sub.GetID())

		i.wg.Done()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case round := <-sub.GetCh():
			isValidPrePrepare := func(message *proto.Message) bool {
				// Extract the payload data
				proposal := messages.ExtractProposal(message)
				proposalHash := messages.ExtractProposalHash(message)
				certificate := messages.ExtractRoundChangeCertificate(message)

				// Verify that the message is indeed from the proposer for this view
				if !i.backend.IsProposer(message.From, height, round) {
					return false
				}

				// Verify that the proposal hash matches the proposal
				if !i.backend.IsValidProposalHash(proposal, proposalHash) {
					return false
				}

				// Make sure there is a certificate
				if certificate == nil {
					return false
				}

				// Make sure there are Quorum RCC
				if len(certificate.RoundChangeMessages) < quorum {
					return false
				}

				// Make sure the current node is not the proposer for this round
				if i.backend.IsProposer(i.backend.ID(), height, round) {
					return false
				}

				// Make sure all messages in the RCC are valid Round Change messages
				for _, rc := range certificate.RoundChangeMessages {
					// Make sure the message is a Round Change message
					if rc.Type != proto.MessageType_ROUND_CHANGE {
						return false
					}
				}

				return true
			}

			msgs := i.messages.GetValidMessages(
				&proto.View{
					Height: height,
					Round:  round,
				},
				proto.MessageType_PREPREPARE,
				isValidPrePrepare,
			)

			// Extract the proposal
			proposalMessage := msgs[0]
			rcc := messages.ExtractRoundChangeCertificate(proposalMessage)
			proposalHash := messages.ExtractProposalHash(proposalMessage)
			proposal := messages.ExtractProposal(proposalMessage)

			// Extract possible rounds and their corresponding
			// block hashes
			type roundHashTuple struct {
				round uint64
				hash  []byte
			}

			roundsAndPreparedBlockHashes := make([]roundHashTuple, 0)

			for _, rcMessage := range rcc.RoundChangeMessages {
				certificate := messages.ExtractLatestPC(rcMessage)

				// Check if there is a certificate, and if it's a valid PC
				if certificate != nil && i.validPC(certificate, proposalMessage.View.Round) {
					hash := messages.ExtractProposalHash(certificate.ProposalMessage)

					roundsAndPreparedBlockHashes = append(roundsAndPreparedBlockHashes, roundHashTuple{
						round: rcMessage.View.Round,
						hash:  hash,
					})
				}
			}

			if len(roundsAndPreparedBlockHashes) > 0 {
				// Find the max round
				var (
					maxRound     uint64 = 0
					expectedHash []byte = nil
				)

				for _, tuple := range roundsAndPreparedBlockHashes {
					if tuple.round > maxRound {
						maxRound = tuple.round
						expectedHash = tuple.hash
					}
				}

				if bytes.Equal(expectedHash, proposalHash) {
					// The proposal contains a verifiable old proposal, it's valid
					i.signalNewProposal(ctx, newProposalEvent{
						proposal: proposal,
						round:    round,
					})

					return
				}
			} else {
				if i.backend.IsValidBlock(proposal) {
					// The proposal contains a newly created proposal, it's valid
					i.signalNewProposal(ctx, newProposalEvent{
						proposal: proposal,
						round:    round,
					})

					return
				}
			}
		}
	}
}

func (i *IBFT) watchForRoundChangeCertificates(ctx context.Context) {
	var (
		view = i.state.getView()

		sub = i.messages.Subscribe(
			messages.SubscriptionDetails{
				MessageType: proto.MessageType_ROUND_CHANGE,
				View: &proto.View{
					Height: view.Height,
					Round:  view.Round + 1,
				},
				NumMessages: 1,
			},
		)
	)

	defer func() {
		i.messages.Unsubscribe(sub.GetID())

		i.wg.Done()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-sub.GetCh(): //	TODO: handle
		}
	}
}

func (i *IBFT) RunSequencee(ctx context.Context, h uint64) {
	// Set the starting state data
	i.state.clear(h)
	i.messages.PruneByHeight(i.state.getHeight())

	i.log.Info("sequence started", "height", h)
	defer i.log.Info("sequence complete", "height", h)

	for {
		i.wg.Add(5)

		currentRound := i.state.getRound()

		ctxRound, cancelRound := context.WithCancel(ctx)

		// Start the round timer worker
		go i.startRoundTimer(ctxRound, currentRound, roundZeroTimeout)

		// Start the round hop worker
		go i.watchForRoundHop(ctxRound)

		//	Jump round on proposals from higher rounds
		go i.watchForFutureProposal(ctxRound)

		//	Jump round on certificates
		go i.watchForRoundChangeCertificates(ctxRound)

		// Start the state machine worker
		go i.runRound(ctxRound)

		teardown := func() {
			cancelRound()
			i.wg.Wait()
		}

		select {
		case _ = <-i.newProposal:
		//	TODO: handle future proposal transition
		case _ = <-i.roundCertificate:
		//	TODO: handle round certificate transition
		case newRound := <-i.roundTimer:
			// Round Change request received.
			// Stop all running worker threads
			i.log.Info("round change received")

			teardown()

			//	Move to the new round
			i.moveToNewRoundWithRC(newRound)
			i.state.setLocked(false)

			i.log.Info("going to round change...")

			// Wait to reach quorum on what the next round
			// should be before starting the cycle again
			i.runRoundChange()
		case <-i.roundDone:
			// The consensus cycle for the block height is finished.
			// Stop all running worker threads
			teardown()

			return
		case <-ctx.Done():
			teardown()

			return
		}
	}
}

// RunSequence runs the consensus cycle for the specified block height
func (i *IBFT) RunSequence(ctx context.Context, h uint64) {
	// Set the starting state data
	i.state.clear(h)
	i.messages.PruneByHeight(i.state.getHeight())

	i.log.Info("sequence started", "height", h)
	defer i.log.Info("sequence complete", "height", h)

	for {
		i.wg.Add(3)

		currentRound := i.state.getRound()

		ctxRound, cancelRound := context.WithCancel(ctx)

		// Start the round timer worker
		go i.startRoundTimer(ctxRound, currentRound, roundZeroTimeout)

		// Start the state machine worker
		go i.runRound(ctxRound)

		// Start the round hop worker
		go i.watchForRoundHop(ctxRound)

		select {
		case newRound := <-i.roundTimer:
			// Round Change request received.
			// Stop all running worker threads
			i.log.Info("round change received")

			cancelRound()
			i.wg.Wait()

			//	Move to the new round
			i.moveToNewRoundWithRC(newRound)
			i.state.setLocked(false)

			i.log.Info("going to round change...")

			// Wait to reach quorum on what the next round
			// should be before starting the cycle again
			i.runRoundChange()
		case <-i.roundDone:
			// The consensus cycle for the block height is finished.
			// Stop all running worker threads
			cancelRound()
			i.wg.Wait()

			return
		case <-ctx.Done():
			cancelRound()
			i.wg.Wait()

			return
		}
	}
}

// runRound runs the state machine loop for the current round
func (i *IBFT) runRound(ctx context.Context) {
	// Register this worker thread with the barrier
	defer i.wg.Done()

	if !i.state.isRoundStarted() {
		// Round is not yet started, kick the round off
		i.state.changeState(newRound)
		i.state.setRoundStarted(true)

		i.log.Info("round started", "round", i.state.getRound())
	}

	var (
		id     = i.backend.ID()
		height = i.state.getHeight()
		round  = i.state.getRound()
	)

	// Check if any block needs to be proposed
	if i.backend.IsProposer(id, height, round) {
		i.log.Info("we are the proposer")

		if round == 0 {
			// TODO see which context to use
			if err := i.proposeBlock(context.TODO(), height, 0); err != nil {
				// Proposal is unable to be submitted, move to the round change state
				i.log.Error("unable to propose block, alerting of round change")

				i.signalRoundChange(ctx, round+1)

				return
			}
		} else {
			//	round > 0 -> needs RCC
			rcc := i.waitForRCC(ctx, height, round)
			if rcc == nil {
				// Timeout occurred
				return
			}

			//	check the messages for any previous proposal (if they have any, it's the same proposal)
			var previousProposal []byte

			for _, msg := range rcc.RoundChangeMessages {
				//	if message contains block, break
				latestPC := messages.ExtractLatestPC(msg)

				if latestPC != nil {
					previousProposal = messages.ExtractLastPreparedProposedBlock(msg)

					break
				}
			}

			var proposal []byte
			if previousProposal == nil {
				//	build new proposal
				var err error

				proposal, err = i.buildProposal(height)
				if err != nil {
					// Proposal is unable to be built
					i.log.Error("unable to build proposal")

					i.signalRoundChange(ctx, round+1)

					return
				}
			} else {
				proposal = previousProposal
			}

			i.acceptProposal(proposal)
			i.log.Debug("block proposal accepted")

			i.transport.Multicast(
				i.backend.BuildPrePrepareMessage(proposal, i.state.getView()),
			)

			i.log.Debug("pre-prepare message multicasted")
		}
	}

	i.runStates(ctx)
}

func (i *IBFT) waitForRCC(ctx context.Context, height, round uint64) *proto.RoundChangeCertificate {
	quorum := i.backend.Quorum(height)
	view := &proto.View{
		Height: height,
		Round:  round,
	}

	sub := i.messages.Subscribe(
		messages.SubscriptionDetails{
			MessageType: proto.MessageType_ROUND_CHANGE,
			View:        view,
			NumMessages: int(quorum),
		},
	)

	defer i.messages.Unsubscribe(sub.GetID())

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-sub.GetCh():
			rcc := i.handleRoundChangeMessage(view, quorum)
			if rcc == nil {
				continue
			}

			return rcc
		}
	}
}

func (i *IBFT) handleRoundChangeMessage(view *proto.View, quorum uint64) *proto.RoundChangeCertificate {
	isValidFn := func(msg *proto.Message) bool {
		proposal := messages.ExtractLastPreparedProposedBlock(msg)
		certificate := messages.ExtractLatestPC(msg)

		if !i.validPC(certificate, view.Round) {
			return false
		}

		return i.matchProposalWithCertificate(proposal, certificate)
	}

	msgs := i.messages.GetValidMessages(
		view,
		proto.MessageType_ROUND_CHANGE,
		isValidFn,
	)

	if len(msgs) < int(quorum) {
		return nil
	}

	return &proto.RoundChangeCertificate{
		RoundChangeMessages: msgs,
	}
}

func (i *IBFT) matchProposalWithCertificate(proposal []byte, certificate *proto.PreparedCertificate) bool {
	if proposal == nil && certificate == nil {
		return true
	}

	if certificate == nil {
		return false
	}

	hashesInCertificate := make([][]byte, 0)

	//	collect hash from pre-prepare message
	proposalHash := messages.ExtractProposalHash(certificate.ProposalMessage)
	hashesInCertificate = append(hashesInCertificate, proposalHash)

	//	collect hashes from prepare messages
	for _, msg := range certificate.PrepareMessages {
		proposalHash := messages.ExtractPrepareHash(msg)

		hashesInCertificate = append(hashesInCertificate, proposalHash)
	}

	//	verify all hashes match the proposal
	for _, hash := range hashesInCertificate {
		if !i.backend.IsValidProposalHash(proposal, hash) {
			return false
		}
	}

	return true
}

//	runStates is the main loop which performs state transitions
func (i *IBFT) runStates(ctx context.Context) {
	var err error

	for {
		switch i.state.getStateName() {
		case newRound:
			err = i.runNewRound(ctx)
		case prepare:
			err = i.runPrepare(ctx)
		case commit:
			err = i.runCommit(ctx)
		case fin:
			if err = i.runFin(); err == nil {
				//	Block inserted without any errors,
				// sequence is complete
				//	TODO: we managed to insert the block, but timeout expired
				//		We need to stop the timer when entering Fin state
				select {
				case i.roundDone <- struct{}{}:
				case <-ctx.Done():
				}

				return
			}
		}

		if errors.Is(err, errTimeoutExpired) {
			i.log.Info("round timeout expired")

			return
		}

		if err != nil {
			// There was a critical consensus error during
			// state execution, move to the round change state
			i.log.Error(
				"error during state processing",
				"state", i.state.getStateName().String(),
				"err", err,
			)

			i.signalRoundChange(ctx, i.state.getRound()+1)

			return
		}
	}
}

// runNewRound runs the New Round IBFT state
func (i *IBFT) runNewRound(ctx context.Context) error {
	i.log.Debug("enter: new round state")
	defer i.log.Debug("exit: new round state")

	var (
		// Grab the current view
		view = i.state.getView()

		// Subscribe for PREPREPARE messages
		sub = i.messages.Subscribe(
			messages.SubscriptionDetails{
				MessageType: proto.MessageType_PREPREPARE,
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
		case <-ctx.Done():
			// Stop signal received, exit
			return errTimeoutExpired
		case <-sub.GetCh():
			// SubscriptionDetails conditions have been met,
			// grab the proposal messages
			return i.handlePrePrepare(view)
		}
	}
}

//	handlePrePrepare parses the received proposal and performs
//	a transition to PREPARE state, if the proposal is valid
func (i *IBFT) handlePrePrepare(view *proto.View) error {
	proposal := i.getPrePrepareMessage(view)

	//	Validate the proposal
	if err := i.validateProposal(proposal); err != nil {
		return err
	}

	// Accept the proposal since it's valid
	i.acceptProposal(proposal)

	// Multicast the PREPARE message
	i.transport.Multicast(
		i.backend.BuildPrepareMessage(proposal, view),
	)

	i.log.Debug("prepare message multicasted")

	// Move to the prepare state
	i.state.changeState(prepare)

	return nil
}

func (i *IBFT) getPrePrepareMessage(view *proto.View) []byte {
	isValidPrePrepare := func(message *proto.Message) bool {
		// Verify that the message is indeed from the proposer for this view
		return i.backend.IsProposer(message.From, view.Height, view.Round)
	}

	msgs := i.messages.GetValidMessages(
		view,
		proto.MessageType_PREPREPARE,
		isValidPrePrepare,
	)

	return messages.ExtractProposal(msgs[0])
}

// runPrepare runs the Prepare IBFT state
func (i *IBFT) runPrepare(ctx context.Context) error {
	i.log.Debug("enter: prepare state")
	defer i.log.Debug("exit: prepare state")

	var (
		// Grab the current view
		view = i.state.getView()

		// Grab quorum information
		quorum = i.backend.Quorum(view.Height)

		// Subscribe to PREPARE messages
		sub = i.messages.Subscribe(
			messages.SubscriptionDetails{
				MessageType: proto.MessageType_PREPARE,
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
		case <-ctx.Done():
			// Stop signal received, exit
			return errTimeoutExpired
		case <-sub.GetCh():
			if !i.handlePrepare(view, quorum) {
				//	quorum of valid prepare messages not received, retry
				continue
			}

			return nil
		}
	}
}

//	handlePrepare parses available prepare messages and performs
//	a transition to COMMIT state, if quorum was reached
func (i *IBFT) handlePrepare(view *proto.View, quorum uint64) bool {
	isValidPrepare := func(message *proto.Message) bool {
		// Verify that the proposal hash is valid
		return i.backend.IsValidProposalHash(
			i.state.getProposal(),
			messages.ExtractPrepareHash(message),
		)
	}

	if len(
		i.messages.GetValidMessages(
			view,
			proto.MessageType_PREPARE,
			isValidPrepare,
		),
	) < int(quorum) {
		//	quorum not reached, keep polling
		return false
	}

	// Multicast the COMMIT message
	i.transport.Multicast(
		i.backend.BuildCommitMessage(i.state.getProposal(), view),
	)

	i.log.Debug("commit message multicasted")

	// Make sure the node is locked
	i.state.setLocked(true)

	// Move to the commit state
	i.state.changeState(commit)

	return true
}

// runCommit runs the Commit IBFT state
func (i *IBFT) runCommit(ctx context.Context) error {
	i.log.Debug("enter: commit state")
	defer i.log.Debug("exit: commit state")

	var (
		// Grab the current view
		view = i.state.getView()

		// Grab quorum information
		quorum = i.backend.Quorum(view.Height)

		// Subscribe to COMMIT messages
		sub = i.messages.Subscribe(
			messages.SubscriptionDetails{
				MessageType: proto.MessageType_COMMIT,
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
		case <-ctx.Done():
			// Stop signal received, exit
			return errTimeoutExpired
		case <-sub.GetCh():
			if !i.handleCommit(view, quorum) {
				//	quorum not reached, retry
				continue
			}

			return nil
		}
	}
}

//	handleCommit parses available commit messages and performs
//	a transition to FIN state, if quorum was reached
func (i *IBFT) handleCommit(view *proto.View, quorum uint64) bool {
	isValidCommit := func(message *proto.Message) bool {
		var (
			proposalHash  = messages.ExtractCommitHash(message)
			committedSeal = messages.ExtractCommittedSeal(message)
		)
		//	Verify that the proposal hash is valid
		if !i.backend.IsValidProposalHash(i.state.getProposal(), proposalHash) {
			return false
		}

		//	Verify that the committed seal is valid
		return i.backend.IsValidCommittedSeal(proposalHash, committedSeal)
	}

	commitMessages := i.messages.GetValidMessages(view, proto.MessageType_COMMIT, isValidCommit)
	if len(commitMessages) < int(quorum) {
		//	quorum not reached, keep polling
		return false
	}

	// Set the committed seals
	i.state.setCommittedSeals(
		messages.ExtractCommittedSeals(commitMessages),
	)

	//	Move to the fin state
	i.state.changeState(fin)

	return true
}

// runFin runs the fin state (block insertion)
func (i *IBFT) runFin() error {
	i.log.Debug("enter: fin state")
	defer i.log.Debug("exit: fin state")

	// Insert the block to the node's underlying
	// blockchain layer
	if err := i.backend.InsertBlock(
		i.state.getProposal(),
		i.state.getCommittedSeals(),
	); err != nil {
		return errInsertBlock
	}

	// Remove stale messages
	i.messages.PruneByHeight(i.state.getHeight())

	return nil
}

// runRoundChange runs the Round Change IBFT state
func (i *IBFT) runRoundChange() {
	i.log.Debug("enter: round change state")
	defer i.log.Debug("exit: round change state")

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

	i.log.Debug(
		"waiting on quorum of round change messages",
		"height", view.Height,
		"round", view.Round,
		"quorum", quorum,
	)

	ticker := time.NewTicker(roundZeroTimeout / 3)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			//	periodically send the same round change message
			//	to ensure reconnecting nodes are able to catch up
			i.transport.Multicast(
				i.backend.BuildRoundChangeMessage(
					view.Height,
					view.Round,
				),
			)
		case <-sub.GetCh():
			i.log.Debug("received quorum of round change messages")

			return
		}
	}
}

// moveToNewRound moves the state to the new round
func (i *IBFT) moveToNewRound(round uint64) {
	i.state.setView(&proto.View{
		Height: i.state.getHeight(),
		Round:  round,
	})

	i.state.setRoundStarted(false)
	i.state.setProposal(nil)
	i.state.changeState(roundChange)

	i.log.Info("moved to new round", "round", round)
}

// moveToNewRoundWithRC moves the state to the new round change
// and multicasts an appropriate Round Change message
func (i *IBFT) moveToNewRoundWithRC(round uint64) {
	i.moveToNewRound(round)

	i.transport.Multicast(
		i.backend.BuildRoundChangeMessage(
			i.state.getHeight(),
			round,
		),
	)

	i.log.Info(
		"round change message multicasted",
		"sequence", i.state.getHeight(),
		"round", round,
	)
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

//	TODO: revise this
// proposeBlock proposes a block to other peers through multicast
func (i *IBFT) proposeBlock(ctx context.Context, height uint64, round uint64) error {
	proposal, err := i.buildProposal(height)
	if err != nil {
		return err
	}

	i.acceptProposal(proposal)
	i.log.Debug("block proposal accepted")

	i.transport.Multicast(
		i.backend.BuildPrePrepareMessage(proposal, i.state.getView()),
	)

	i.log.Debug("pre-prepare message multicasted")

	i.transport.Multicast(
		i.backend.BuildPrepareMessage(proposal, i.state.getView()),
	)

	i.log.Debug("prepare message multicasted")

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
	i.state.changeState(prepare)
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
	if message.View == nil {
		return false
	}

	// Make sure the message is in accordance with
	// the current state height, or greater
	if i.state.getHeight() > message.View.Height {
		return false
	}

	// Make sure the message round is >= the current state round
	return message.View.Round >= i.state.getRound()
}

//	ExtendRoundTimeout extends each round's timer by the specified amount.
func (i *IBFT) ExtendRoundTimeout(amount time.Duration) {
	i.additionalTimeout = amount
}

// validPC verifies that  the prepared certificate is valid
func (i *IBFT) validPC(certificate *proto.PreparedCertificate, rLimit uint64) bool {
	if certificate == nil {
		// PCs that are not set are valid by default
		return true
	}

	// Make sure that either both the proposal message and the prepare messages are set together
	if certificate.ProposalMessage == nil || certificate.PrepareMessages == nil {
		return false
	}

	allMessages := append(
		[]*proto.Message{certificate.ProposalMessage},
		certificate.PrepareMessages...,
	)

	// Make sure there are at least Quorum (PP + P) messages
	if len(allMessages) < int(i.backend.Quorum(i.state.getHeight())) {
		return false
	}

	// Make sure the proposal message is a Preprepare message
	if certificate.ProposalMessage.Type != proto.MessageType_PREPREPARE {
		return false
	}

	// Make sure all messages in the PC are Prepare messages
	for _, message := range certificate.PrepareMessages {
		if message.Type != proto.MessageType_PREPARE {
			return false
		}
	}

	// Make sure the senders are unique
	if !messages.HasUniqueSenders(allMessages) {
		return false
	}

	// Make sure the proposal hashes match
	if !messages.HaveSameProposalHash(allMessages) {
		return false
	}

	// Make sure all the messages have a round number lower than rLimit
	if !messages.AllHaveLowerRound(allMessages, rLimit) {
		return false
	}

	// Make sure the proposal message is sent by the proposer
	// for the round
	proposal := certificate.ProposalMessage
	if !i.backend.IsProposer(proposal.From, proposal.View.Height, proposal.View.Round) {
		return false
	}

	// Make sure the Prepare messages are validators, apart from the proposer
	for _, message := range certificate.PrepareMessages {
		// Make sure the sender is part of the validator set
		if !i.backend.IsValidSender(message) {
			return false
		}
	}

	return true
}
