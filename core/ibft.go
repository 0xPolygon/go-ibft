package core

import (
	"bytes"
	"errors"
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

	GetMessages(view *proto.View, messageType proto.MessageType) []*proto.Message
	GetProposal(view *proto.View) []byte
	GetCommittedSeals(view *proto.View) [][]byte
	GetMostRoundChangeMessages(minRound, height uint64) []*proto.Message
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

type QuorumFn func(num uint64) uint64

type IBFT struct {
	log Logger

	state *state

	verifiedMessages   Messages
	unverifiedMessages Messages

	backend Backend

	transport Transport

	quorumFn QuorumFn

	roundDone chan event

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
		roundDone:          make(chan event),
		newMessageCh:       make(chan *proto.Message, 1),
		proposalAcceptedCh: make(chan struct{}, 1),
		eventCh:            make(chan event, 1),
		errorCh:            make(chan error, 1),
	}
}

func (i *IBFT) runSequence(h uint64) {
	// Start the message handler thread
	messageHandlerQuit := make(chan struct{})
	go i.runMessageHandler(messageHandlerQuit)

	defer func() {
		messageHandlerQuit <- struct{}{}
	}()

	for {
		currentRound := i.state.getRound()
		quitCh := make(chan struct{})

		go i.runRound(quitCh)

		select {
		case <-i.newRoundTimer(currentRound):
			close(quitCh)

			i.moveToNewRoundWithRC(i.state.getRound()+1, i.state.getHeight())
			i.state.setLocked(false)
		case event := <-i.roundDone:
			i.stopRoundTimeout()
			close(quitCh)

			if event == consensusReached {
				// Sequence is finished, exit
				return
			}
		}
	}
}

// newRoundTimer instantiates a new exponential round timer
func (i *IBFT) newRoundTimer(round uint64) <-chan time.Time {
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

// runRound is the main run loop for the IBFT round
func (i *IBFT) runRound(quit <-chan struct{}) {
	// Set the initial state
	i.state.setStateName(newRound)
	i.state.setRoundStarted(true)

	// Propose the block if proposer
	if i.backend.IsProposer(
		i.backend.ID(),
		i.state.getHeight(),
		i.state.getRound(),
	) {
		// TODO should this emit a proposal accepted event to the event handler?
		if err := i.proposeBlock(i.state.getHeight()); err != nil {
			i.moveToNewRoundWithRC(i.state.getRound()+1, i.state.getHeight())
		}
	}

	for {
		select {
		case <-quit:
			return
		case err := <-i.errorCh:
			i.log.Error("error during processing", err)

			if errors.Is(err, errInvalidBlockProposal) || errors.Is(err, errInvalidBlockProposed) {
				i.moveToNewRoundWithRC(i.state.getRound()+1, i.state.getHeight())
			}
		case event := <-i.eventCh:
			// New event received from the message handler, parse it
			switch event {
			case proposalReceived:
				// Make sure no block is accepted yet
				if i.state.getProposal() != nil {
					// Ignore any kind of additional proposal once one has been accepted
					continue
				}

				proposal := i.verifiedMessages.GetProposal(i.state.getView())
				if proposal == nil {
					// TODO this is not possible?
					i.moveToNewRoundWithRC(i.state.getRound()+1, i.state.getHeight())
				}

				i.acceptProposal(proposal)

				i.transport.Multicast(
					i.backend.BuildPrepareMessage(proposal, i.state.getView()),
				)

				// Alert the message handler that
				// unverified messages can now be verified
				i.proposalAcceptedCh <- struct{}{}
			case quorumPrepares:
				// Make sure there is currently no locked block
				if i.state.isLocked() {
					continue
				}

				i.state.setStateName(commit)
				i.state.setLocked(true)

				i.transport.Multicast(
					i.backend.BuildCommitMessage(i.state.getProposal(), i.state.getView()),
				)
			case quorumCommits:
				// Extract the committed seals
				committedSeals := i.verifiedMessages.GetCommittedSeals(i.state.getView())

				i.state.addCommittedSeals(committedSeals)

				i.state.setStateName(fin)

				if err := i.runFin(); err != nil {
					i.moveToNewRoundWithRC(i.state.getRound()+1, i.state.getHeight())
					i.state.setLocked(false)

					continue
				}

				// Signalize finish
				// TODO also return?
				i.roundDone <- consensusReached
			case quorumRoundChanges:
				// TODO get this data as part of the event?
				msgs := i.verifiedMessages.GetMessages(i.state.getView(), proto.MessageType_ROUND_CHANGE)
				i.state.setRound(msgs[0].View.Round)

				i.roundDone <- repeatSequence
			case roundHop:
				// TODO get this data as part of the event?
				msgs := i.verifiedMessages.GetMostRoundChangeMessages(i.state.getRound()+1, i.state.getHeight())
				suggestedRound := msgs[0].View.Round

				i.moveToNewRoundWithRC(suggestedRound, i.state.getHeight())
			default:
			}
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
}

// moveToNewRoundWithRC moves the state to the new round change
// and multicasts an appropriate Round Change message
func (i *IBFT) moveToNewRoundWithRC(round, height uint64) {
	i.moveToNewRound(round, height)

	i.transport.Multicast(
		i.backend.BuildRoundChangeMessage(
			i.state.getHeight(),
			i.state.getRound(),
		),
	)
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

	i.transport.Multicast(
		i.backend.BuildPrePrepareMessage(proposal, i.state.getView()),
	)

	i.transport.Multicast(
		i.backend.BuildPrepareMessage(proposal, i.state.getView()),
	)

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

	return true
}

// validateMessage does deep message validation based on its type
func (i *IBFT) validateMessage(message *proto.Message) error {
	// The validity of message senders should be
	// confirmed outside this method call, as this method
	// only validates the message contents
	viewsMatch := func(a, b *proto.View) bool {
		return a.Height == b.Height && a.Round == b.Round
	}

	switch message.Type {
	case proto.MessageType_PREPREPARE:
		//	#1: matches current view
		if !viewsMatch(i.state.getView(), message.View) {
			return errViewMismatch
		}

		//	#2:	signed by the designated proposer for this round
		if !i.backend.IsProposer(message.From, i.state.getHeight(), i.state.getRound()) {
			return errInvalidProposer
		}

		//	#3:	accepted proposal == false
		messageProposal := message.Payload.(*proto.Message_PreprepareData).PreprepareData.Proposal

		// Validate that the proposal is correct
		return i.validateProposal(messageProposal)
	case proto.MessageType_PREPARE:
		//	#1: matches current view
		if !viewsMatch(i.state.getView(), message.View) {
			return errViewMismatch
		}

		//	#2:	kec(proposal) == kec(prepared-block)
		proposalHash := message.Payload.(*proto.Message_PrepareData).PrepareData.ProposalHash
		if err := i.backend.VerifyProposalHash(i.state.getProposal(), proposalHash); err != nil {
			return errHashMismatch
		}
	case proto.MessageType_COMMIT:
		//	#1: matches current view
		if !viewsMatch(i.state.getView(), message.View) {
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
		if i.state.getHeight() != message.View.Height {
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
	var (
		numValidators = i.backend.ValidatorCount(i.state.getHeight())
		quorum        = int(i.quorumFn(numValidators))
	)

	switch messageType {
	case proto.MessageType_PREPREPARE:
		return proposalReceived
	case proto.MessageType_PREPARE:
		// Check if there are enough messages
		numPrepares := i.verifiedMessages.NumMessages(i.state.getView(), proto.MessageType_PREPARE)
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
		numCommits := i.verifiedMessages.NumMessages(i.state.getView(), proto.MessageType_COMMIT)

		// Extract the committed seals since we know they're valid
		// at this point
		if numCommits >= quorum {
			return quorumCommits
		}

		numPrepares := i.verifiedMessages.NumMessages(i.state.getView(), proto.MessageType_PREPARE)
		if numCommits+numPrepares >= quorum {
			return quorumPrepares
		}
	case proto.MessageType_ROUND_CHANGE:
		view := i.state.getView()
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
