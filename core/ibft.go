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
	GetAndPrunePrepareMessages(view *proto.View) []*proto.Message
	GetAndPruneCommitMessages(view *proto.View) []*proto.Message
	GetCommitMessages(view *proto.View) []*messages.CommitMessage
	GetRoundChangeMessages(view *proto.View) []*messages.RoundChangeMessage
	GetMostRoundChangeMessages() []*messages.RoundChangeMessage
}

var (
	errBuildProposal        = errors.New("failed to build proposal")
	errInvalidBlockProposal = errors.New("invalid block proposal")
	errInvalidCommittedSeal = errors.New("invalid commit seal in commit message")
	errInsertBlock          = errors.New("failed to insert block")
	errViewMismatch         = errors.New("invalid message view")
	errHashMismatch         = errors.New("data hash mismatch")

	roundZeroTimeout = 10 * time.Second
)

type QuorumFn func(num uint64) uint64

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
	view *proto.View

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

	verifiedMessages   Messages
	unverifiedMessages Messages

	backend Backend

	transport Transport

	quorumFn QuorumFn

	roundDone chan error

	roundTimer *time.Timer

	// IBFT -> message handler
	newMessageCh  chan *proto.Message
	stateChangeCh chan stateName

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
		roundDone: make(chan error),
	}
}

func (i *IBFT) runSequence(h uint64) {
	for {
		currentRound := i.state.view.Round
		quitCh := make(chan struct{})

		go i.runRound(quitCh)

		select {
		case <-i.newRoundTimer(currentRound):
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
	}
}

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

func (i *IBFT) runRound(quit <-chan struct{}) {
	i.state.name = newRound
	i.state.roundStarted = true

	// Propose the block if proposer
	if i.backend.IsProposer(
		i.backend.ID(),
		i.state.view.Height,
		i.state.view.Round,
	) {
		if err := i.proposeBlock(i.state.view.Height); err != nil {
			// TODO handle
			panic("bad")
		}
	}

	for {
		select {
		case <-quit:
			return
		case err := <-i.errorCh:
			// TODO handle error
			if errors.Is(err, errInvalidBlockProposal) {

			}
		case event := <-i.eventCh:
			// New event received, parse it
			switch event {
			case proposalReceived:
				preprepareMsg := i.verifiedMessages.GetPrePrepareMessage(i.state.view)
				if preprepareMsg == nil {
					panic("bad")
				}

				newProposal := preprepareMsg.Proposal
				i.acceptProposal(newProposal)

				i.transport.Multicast(
					i.backend.BuildPrepareMessage(newProposal),
				)
			case quorumPrepares:
				i.state.name = commit
				i.state.locked = true

				i.transport.Multicast(
					i.backend.BuildCommitMessage(i.state.proposal),
				)
			case quorumCommits:
				//	get commit messages
				commitMessages := i.commitMessages(i.state.view)
				//	add seals
				for _, msg := range commitMessages {
					//	TODO: these need to be pruned before each new round
					i.state.seals = append(i.state.seals, msg.CommittedSeal)
				}

				//	block proposal finalized -> fin state
				i.state.name = fin
			case quorumRoundChanges:
				// TODO pass this as an argument to the method
				msgs := i.verifiedMessages.GetMostRoundChangeMessages()
				newRound := msgs[0].Round

				i.state.view.Round = newRound
			case roundHop:
				// TODO pass this as an argument to the method
				msgs := i.verifiedMessages.GetMostRoundChangeMessages()
				suggestedRound := msgs[0].Round

				i.state.view.Round = suggestedRound
				i.state.proposal = nil
				i.state.roundStarted = false

				i.transport.Multicast(
					i.backend.BuildRoundChangeMessage(
						i.state.view.Height,
						suggestedRound,
					))

				i.roundDone <- errors.New("higher round change received")

				// TODO exit??
			default:
			}
		}

		// TODO FIN state
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

	for _, msg := range i.verifiedMessages.GetCommitMessages(view) {
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

func (i *IBFT) prepareMessages(view *proto.View) []*messages.PrepareMessage {
	var valid []*messages.PrepareMessage

	for _, msg := range i.verifiedMessages.GetPrepareMessages(view) {
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
		return errInvalidBlockProposal
	}

	if !i.backend.IsValidBlock(newProposal) {
		return errInvalidBlockProposal

	}

	return nil
}

func (i *IBFT) acceptProposal(proposal []byte) {
	//	accept newly proposed block and move to PREPARE state
	i.state.proposal = proposal
	i.state.name = prepare
}

func (i *IBFT) AddMessage(message *proto.Message) {
	// Make sure the message is present
	if message == nil {
		return
	}

	if i.isAcceptableMessage(message) {
		// Message is valid, alert the message handler
		// TODO make async
		i.newMessageCh <- message
	}
}

func (i *IBFT) isAcceptableMessage(message *proto.Message) bool {
	//	Make sure the message sender is ok
	if !i.backend.IsValidMessage(message) {
		return false
	}

	// Invalid messages are discarded
	// TODO move to specific format checker method
	if message.View == nil {
		return false
	}

	// Make sure the message is in accordance with
	// the current state height
	if i.state.view.Height != message.View.Height {
		return false
	}

	// Make sure the message round is >= the current state round
	return message.View.Round >= i.state.view.Round
}

func (i *IBFT) canVerifyMessage(message *proto.Message) bool {
	// Round change messages can always be validated
	if message.Type == proto.MessageType_ROUND_CHANGE {
		return true
	}

	// PREPARE and COMMIT messages can be verified after PREPREPARE, but not before
	// PREPARE and COMMIT messages can be verified independently of one another!
	return i.state.name == newRound && message.Type != proto.MessageType_PREPREPARE
}

func viewsMatch(a, b *proto.View) bool {
	return a.Height == b.Height && a.Round == b.Round
}

func (i *IBFT) validateMessage(message *proto.Message) error {
	switch message.Type {
	case proto.MessageType_PREPREPARE:
		/*	PRE-PREPARE	*/
		//	#1: matches current view
		if !viewsMatch(i.state.view, message.View) {
			return errViewMismatch
		}

		//	#2:	signed by the designated proposer for this round
		if !i.backend.IsProposer(message.From, i.state.view.Height, i.state.view.Round) {
			return errInvalidBlockProposal
		}

		//	#3:	accepted proposal == false
		messageProposal := message.Payload.(*proto.Message_PreprepareData).PreprepareData.Proposal

		if err := i.validateProposal(messageProposal); err != nil {
			return errInvalidBlockProposal
		}
	case proto.MessageType_PREPARE:
		/*	PREPARE	*/
		//	#1: matches current view
		if !viewsMatch(i.state.view, message.View) {
			return errViewMismatch
		}

		//	#2:	kec(proposal) == kec(prepared-block)
		proposalHash := message.Payload.(*proto.Message_PrepareData).PrepareData.ProposalHash
		if err := i.backend.VerifyProposalHash(i.state.proposal, proposalHash); err != nil {
			return errHashMismatch
		}
	case proto.MessageType_COMMIT:
		/*	COMMIT	*/
		//	#1: matches current view
		if !viewsMatch(i.state.view, message.View) {
			return errViewMismatch
		}

		//	#2:	kec(proposal) == kec(accepted-block)
		proposalHash := message.Payload.(*proto.Message_CommitData).CommitData.ProposalHash
		if err := i.backend.VerifyProposalHash(i.state.proposal, proposalHash); err != nil {
			return errHashMismatch
		}

		// #3: valid committed seal
		committedSeal := message.Payload.(*proto.Message_CommitData).CommitData.CommittedSeal
		if !i.backend.IsValidCommittedSeal(proposalHash, committedSeal) {
			return errInvalidCommittedSeal
		}
	case proto.MessageType_ROUND_CHANGE:
		/*	ROUND-CHANGE	*/
		//	#1: matches current **height** (round can be greater)
		if i.state.view.Height != message.View.Height {
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
	noEvent
)

func (i *IBFT) eventPossible(messageType proto.MessageType) event {
	var (
		view          = i.state.view
		numValidators = i.backend.ValidatorCount(view.Height)
		quorum        = int(i.quorumFn(numValidators))
	)

	switch messageType {
	case proto.MessageType_PREPREPARE:
		return proposalReceived
	case proto.MessageType_PREPARE:
		// Check if there are enough messages
		// TODO Q(P + C)
		if len(i.prepareMessages(view)) >= quorum {
			return quorumPrepares
		}
	case proto.MessageType_COMMIT:
		if len(i.commitMessages(view)) >= quorum {
			return quorumCommits
		}
	case proto.MessageType_ROUND_CHANGE:
		msgs := i.verifiedMessages.GetMostRoundChangeMessages()

		// Check for Q(RC)
		if len(msgs) >= int(i.quorumFn(i.backend.ValidatorCount(
			i.state.view.Height))) {
			return quorumRoundChanges
		}

		// Check for F+1
		if len(msgs) > 0 {
			suggestedRound := msgs[0].Round
			if i.state.view.Round < suggestedRound && len(msgs) > int(i.backend.AllowedFaulty())+1 {
				//		move to new round
				return roundHop
			}
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

				i.errorCh <- err

				continue
			}

			// Since the message is valid, add it to the verified messages
			i.verifiedMessages.AddMessage(message)

			// Check if any conditions can be met based on the message type
			if event := i.eventPossible(message.Type); event != noEvent {
				i.eventCh <- event
			}
		case newState := <-i.stateChangeCh:
			// A state change occurred, check if any messages can be verified now
			switch newState {
			case prepare:
				// FOR EVERY PREPARE MESSAGE IN UNVERIFIED
				// The message can be verified now

				// TODO this should effectively wipe out the messages as it takes them
				preparedMessages := i.unverifiedMessages.GetAndPrunePrepareMessages(i.state.view)

				for _, prepareMessage := range preparedMessages {
					if err := i.validateMessage(prepareMessage); err != nil {
						// Message is invalid, log it
						i.log.Debug("received invalid message")

						continue
					}

					// Since the message is valid, add it to the verified stack
					i.verifiedMessages.AddMessage(prepareMessage)

					// Check if any conditions can be met based on the message type
					if event := i.eventPossible(prepareMessage.Type); event != noEvent {
						i.eventCh <- event

						// TODO check if this is valid
						return
					}
				}
			case commit:
				// FOR EVERY COMMIT MESSAGE IN UNVERIFIED
				// The message can be verified now

				// TODO this should effectively wipe out the messages as it takes them
				commitMessages := i.unverifiedMessages.GetAndPruneCommitMessages(i.state.view)

				for _, commitMessage := range commitMessages {
					if err := i.validateMessage(commitMessage); err != nil {
						// Message is invalid, log it
						i.log.Debug("received invalid message")

						continue
					}

					// Since the message is valid, add it to the verified stack
					i.verifiedMessages.AddMessage(commitMessage)

					// Check if any conditions can be met based on the message type
					if event := i.eventPossible(commitMessage.Type); event != noEvent {
						i.eventCh <- event

						// TODO check if this is valid
						return
					}
				}
			}
		}
	}
}
