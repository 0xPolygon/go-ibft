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

	verifiedMessages   Messages
	unverifiedMessages Messages

	backend Backend

	transport Transport

	roundDone   chan error
	roundChange chan uint64

	wg sync.WaitGroup

	roundTimer *time.Timer

	// IBFT -> message handler
	newMessageCh       chan *proto.Message
	proposalAcceptedCh chan struct{} // TODO rename this to something more appropriate

	// message handler -> IBFT
	eventCh chan event
	errorCh chan error
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
		roundDone:          make(chan error),
		newMessageCh:       make(chan *proto.Message, 1),
		proposalAcceptedCh: make(chan struct{}, 1),
		eventCh:            make(chan event, 1),
		errorCh:            make(chan error, 1),
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
		rcMessages := i.verifiedMessages.
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

		//	TODO
		/*	ROUND CHANGE state	*/

		//	this is where we wait on quorum RC messages
		//	before moving on with the for loop...
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
			_ = i.runPrepare(quit)
		case commit:
			_ = i.runCommit(quit)
		case fin:
			_ = i.runFin()
		}

	}
}

func (i *IBFT) runNewRound(quit <-chan struct{}) error {
	view := i.state.getView()
	sub := i.verifiedMessages.Subscribe(
		messages.SubscriptionDetails{
			MessageType: proto.MessageType_PREPREPARE,
			View:        view,
			NumMessages: 1,
		})

	defer i.verifiedMessages.Unsubscribe(sub.GetID())

	for {
		select {
		case <-quit:
			return errors.New("round timeout expired")
		case <-sub.GetCh():
			var proposal []byte

			msgs := i.verifiedMessages.GetMessages(view, proto.MessageType_PREPREPARE)
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

	sub := i.verifiedMessages.Subscribe(
		messages.SubscriptionDetails{
			MessageType: proto.MessageType_PREPARE,
			View:        view,
			NumMessages: int(quorum),
		})

	defer i.verifiedMessages.Unsubscribe(sub.GetID())

	for {
		select {
		case <-quit:
			return errors.New("round timeout expired")
		case <-sub.GetCh():
			//	TODO
			//	get messages

			//	validation
			//		#1:	kec(proposal) == kec(prepare)

			//	multicast COMMIT message

			//	move to commit state and return
		}
	}
}

func (i *IBFT) runCommit(quit <-chan struct{}) error {
	var (
		view   = i.state.getView()
		quorum = i.backend.Quorum(view.Height)
	)

	sub := i.verifiedMessages.Subscribe(
		messages.SubscriptionDetails{
			MessageType: proto.MessageType_COMMIT,
			View:        view,
			NumMessages: int(quorum),
		})

	defer i.verifiedMessages.Unsubscribe(sub.GetID())

	for {
		select {
		case <-quit:
			return errors.New("round timeout expired")
		case <-sub.GetCh():
			//	TODO
			//	get messages

			//	validation
			//		#1:	kec(proposal) == kec(commit)
			//		#2:	commited seal is ok

			//	move to FIN state and return
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
	i.verifiedMessages.PruneByHeight(view)
	i.unverifiedMessages.PruneByHeight(view)

	return nil
}

func (i *IBFT) startNewRound() {
	i.state.setStateName(newRound)
	i.state.setRoundStarted(true)

	// Propose the block if proposer
	if i.backend.IsProposer(
		i.backend.ID(),
		i.state.getHeight(),
		i.state.getRound(),
	) {
		if err := i.proposeBlock(i.state.getHeight()); err != nil {
			i.roundDone <- err
		}
	}
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
		// Message is valid, alert the message handler
		go func() {
			i.newMessageCh <- message
		}()
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
	if i.state.getHeight() != message.View.Height {
		return false
	}

	// Make sure the message round is >= the current state round
	return message.View.Round >= i.state.getRound()
}

// canVerifyMessage checks if the message is currently verifiable
func (i *IBFT) canVerifyMessage(message *proto.Message) bool {
	// Round change messages can always be verified
	if message.Type == proto.MessageType_ROUND_CHANGE {
		return true
	}

	// PREPARE and COMMIT messages can be verified after PREPREPARE, but NOT before
	// PREPARE and COMMIT messages can be verified independently of one another!
	if i.state.getStateName() == newRound {
		return message.Type == proto.MessageType_PREPREPARE
	}

	// Until the round is started, only RC messages can be verified
	if i.state.getStateName() == roundChange {
		return message.Type == proto.MessageType_ROUND_CHANGE
	}

	return true
}

// validateMessage does deep message validation based on its type
func (i *IBFT) validateMessage(message *proto.Message) error {
	view := i.state.getView()
	// The validity of message senders should be
	// confirmed outside this method call, as this method
	// only validates the message contents
	viewsMatch := func(a, b *proto.View) bool {
		return (a.Height == b.Height) && (a.Round == b.Round)
	}

	switch message.Type {
	case proto.MessageType_PREPREPARE:
		//	#1: matches current view
		if !viewsMatch(view, message.View) {
			return errViewMismatch
		}

		//	#2:	signed by the designated proposer for this round
		if !i.backend.IsProposer(message.From, view.Height, view.Round) {
			return errInvalidProposer
		}

		//	#3:	accepted proposal == false
		messageProposal := message.Payload.(*proto.Message_PreprepareData).PreprepareData.Proposal

		// Validate that the proposal is correct
		return i.validateProposal(messageProposal)
	case proto.MessageType_PREPARE:
		//	#1: matches current view
		if !viewsMatch(view, message.View) {
			return errViewMismatch
		}

		//	#2:	kec(proposal) == kec(prepared-block)
		proposalHash := message.Payload.(*proto.Message_PrepareData).PrepareData.ProposalHash
		if err := i.backend.VerifyProposalHash(i.state.getProposal(), proposalHash); err != nil {
			return errHashMismatch
		}
	case proto.MessageType_COMMIT:
		//	#1: matches current view
		if !viewsMatch(view, message.View) {
			return errViewMismatch
		}

		//	#2:	kec(proposal) == kec(accepted-block)
		proposalHash := message.Payload.(*proto.Message_CommitData).CommitData.ProposalHash
		if err := i.backend.VerifyProposalHash(i.state.getProposal(), proposalHash); err != nil {
			return errHashMismatch
		}

		// #3: valid committed seal
		committedSeal := message.Payload.(*proto.Message_CommitData).CommitData.CommittedSeal
		if !i.backend.IsValidCommittedSeal(proposalHash, committedSeal) {
			return errInvalidCommittedSeal
		}
	case proto.MessageType_ROUND_CHANGE:
		//	#1: matches current **height** (round can be greater)
		if view.Height != message.View.Height {
			return errViewMismatch
		}
	}

	return nil
}

type event int

const (
	proposalReceived event = iota
	quorumPrepares
	quorumCommits
	quorumRoundChanges
	roundHop

	// TODO these should probably be separated out
	// as separate events
	consensusReached
	repeatSequence

	noEvent
)

// eventPossible checks if any kind of event is possible
func (i *IBFT) eventPossible(messageType proto.MessageType) event {
	view := i.state.view
	quorum := int(i.backend.Quorum(view.Height))

	switch messageType {
	case proto.MessageType_PREPREPARE:
		return proposalReceived
	case proto.MessageType_PREPARE:
		// Check if there are enough messages
		numPrepares := i.verifiedMessages.NumMessages(view, proto.MessageType_PREPARE)
		if numPrepares >= quorum {
			return quorumPrepares
		}

		// Check if there are enough prepare and commit messages to form quorum
		numCommits := len(i.state.getCommittedSeals())
		if numCommits+numPrepares >= quorum {
			return quorumPrepares
		}
	case proto.MessageType_COMMIT:
		// Check if there are enough messages
		numCommits := i.verifiedMessages.NumMessages(view, proto.MessageType_COMMIT)

		// Extract the committed seals since we know they're valid
		// at this point
		if numCommits >= quorum {
			return quorumCommits
		}

		numPrepares := i.verifiedMessages.NumMessages(view, proto.MessageType_PREPARE)
		if numCommits+numPrepares >= quorum {
			return quorumPrepares
		}
	case proto.MessageType_ROUND_CHANGE:
		numRoundChange := i.verifiedMessages.NumMessages(view, proto.MessageType_ROUND_CHANGE)

		// Check for Q(RC)
		if numRoundChange >= quorum {
			return quorumRoundChanges
		}

		msgs := i.verifiedMessages.GetMostRoundChangeMessages(view.Round+1, view.Height)
		// Check for F+1
		if len(msgs) >= int(i.backend.AllowedFaulty())+1 {
			return roundHop
		}
	}

	return noEvent
}

// runMessageHandler is the main run loop for
// handling incoming messages, and for verifying
// if certain consensus conditions are met
func (i *IBFT) runMessageHandler(quit <-chan struct{}) {
	// TODO consider the situation where a proposal is received but cannot be verified atm

	for {
		select {
		case <-quit:
			return
		case message := <-i.newMessageCh:
			// New message received, if the message can be verified right now
			if !i.canVerifyMessage(message) {
				// Message can't be verified yet, mark it as unverified
				i.unverifiedMessages.AddMessage(message)

				continue
			}

			// The message can be verified now
			if err := i.validateMessage(message); err != nil {
				// Message is invalid, log it
				i.log.Debug("received invalid message")

				// Alert the main loop of this error,
				// as the error can be consensus-breaking
				i.errorCh <- err

				continue
			}

			// Since the message is valid, add it to the verified messages
			i.verifiedMessages.AddMessage(message)

			// Check if any conditions can be met based on the message type
			if event := i.eventPossible(message.Type); event != noEvent {
				i.eventCh <- event
			}
		case <-i.proposalAcceptedCh:
			// A state change occurred, check if any messages
			// that were previously unverifiable can be verified now
			view := i.state.getView()

			unverifiedMessages := i.unverifiedMessages.GetMessages(view, proto.MessageType_PREPARE)
			unverifiedMessages = append(
				unverifiedMessages,
				i.unverifiedMessages.GetMessages(view, proto.MessageType_COMMIT)...,
			)

			for _, unverifiedMessage := range unverifiedMessages {
				if err := i.validateMessage(unverifiedMessage); err != nil {
					// Message is invalid, but not consensus-breaking
					i.errorCh <- err

					continue
				}

				// Since the message is valid, add it to the verified messages
				i.verifiedMessages.AddMessage(unverifiedMessage)

				// Check if any conditions can be met based on the message type
				if event := i.eventPossible(unverifiedMessage.Type); event != noEvent {
					i.eventCh <- event
				}
			}
		}
	}
}
