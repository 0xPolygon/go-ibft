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

	newMessageCh  chan *proto.Message
	stateChangeCh chan stateName
	eventCh       chan string
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
		view          = i.state.view
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
		view   = i.state.view
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

func (i *IBFT) AddMessage(msg *proto.Message) {
	if msg == nil {
		return
	}

	if i.isAcceptableMessage(msg.View) {
		// Message is valid, alert the message handler
		// TODO make async
		i.newMessageCh <- msg
	}
}

// TODO add additional message structure validation (fields set)
func (i *IBFT) isAcceptableMessage(view *proto.View) bool {
	// Invalid messages are discarded
	if view == nil {
		return false
	}

	// Make sure the message is in accordance to the current
	// state height
	if i.state.view.Height != view.Height {
		return false
	}

	// Make sure the message round is >= the current state round
	return view.Round >= i.state.view.Round
}

func (i *IBFT) canVerifyMessage(message *proto.Message) bool {
	// Round change messages can always be validated
	if message.Type == proto.MessageType_ROUND_CHANGE {
		return true
	}

	// TODO refactor this into a helper

	// PREPARE and COMMIT messages can be verified after PREPREPARE, but not before
	// PREPARE and COMMIT messages can be verified independently of one another!
	if i.state.name == newRound && message.Type != proto.MessageType_PREPREPARE {
		return false
	}

	return true
}

func viewsMatch(a, b *proto.View) bool {
	return a.Height == b.Height && a.Round == b.Round
}

func (i *IBFT) isValidMessage(message *proto.Message) bool {
	// The point of verification

	//	TODO: basic validation (not consensus errors)

	switch message.Type {
	case proto.MessageType_PREPREPARE:
		/*	PRE-PREPARE	*/
		//	#1: matches current view
		if !viewsMatch(i.state.view, message.View) {
			return false
		}

		//	#2:	signed by the designated proposer for this round
		if !i.backend.IsProposer(message.From, i.state.view.Height, i.state.view.Round) {
			return false
		}

		// TODO the following errors should trigger round change (invalid proposal)

		//	#3:	accepted proposal == false
		messageProposal := message.Payload.(*proto.Message_PreprepareData).PreprepareData.Proposal
		if i.state.proposal != nil {
			// TODO: should be checked in consensus?
			//	#4:	if locked, proposal matches locked	(consensus err)
			if !bytes.Equal(i.state.proposal, messageProposal) {
				return false
			}
		}

		// TODO: should be checked in consensus?
		//	#5:	proposal is a valid eth block for this height	(consensus err)
		if !i.backend.IsValidBlock(messageProposal) {
			return false
		}

		//	#6:	message is from validator for this round
		if !i.backend.IsValidMessage(message) {
			return false
		}
	case proto.MessageType_PREPARE:
		/*	PREPARE	*/
		//	#1: matches current view
		if !viewsMatch(i.state.view, message.View) {
			return false
		}

		//	#2:	kec(proposal) == kec(prepared-block)
		proposalHash := message.Payload.(*proto.Message_PrepareData).PrepareData.ProposalHash
		// TODO check if proposal is set...
		if err := i.backend.VerifyProposalHash(i.state.proposal, proposalHash); err != nil {
			return false
		}

		//	#3:	message is from validator for this round
		if !i.backend.IsValidMessage(message) {
			return false
		}
	case proto.MessageType_COMMIT:
		/*	COMMIT	*/
		//	#1: matches current view
		if !viewsMatch(i.state.view, message.View) {
			return false
		}

		//	#2:	kec(proposal) == kec(accepted-block)
		proposalHash := message.Payload.(*proto.Message_CommitData).CommitData.ProposalHash
		// TODO check if proposal is set...
		if err := i.backend.VerifyProposalHash(i.state.proposal, proposalHash); err != nil {
			return false
		}

		//	#3:	message is from validator for this round
		if !i.backend.IsValidMessage(message) {
			return false
		}

		// #4: valid committed seal
		committedSeal := message.Payload.(*proto.Message_CommitData).CommitData.CommittedSeal
		if !i.backend.IsValidCommittedSeal(proposalHash, committedSeal) {
			return false
		}
	case proto.MessageType_ROUND_CHANGE:
		/*	ROUND-CHANGE	*/
		//	#1: matches current **height** (round can be greater)
		if i.state.view.Height != message.View.Height {
			return false
		}

		//	#2:	message is from validator for this round
		if !i.backend.IsValidMessage(message) {
			return false
		}
	}

	return true
}

func (i *IBFT) eventPossible(messageType proto.MessageType) string {
	var (
		view          = i.state.view
		numValidators = i.backend.ValidatorCount(view.Height)
		quorum        = int(i.quorumFn(numValidators))
	)

	switch messageType {
	case proto.MessageType_PREPREPARE:
		return "preprepare received"
	case proto.MessageType_PREPARE:
		// Check if there are enough messages
		// TODO Q(P + C)
		if len(i.prepareMessages(view)) >= quorum {
			return "enough prepares"
		}
	case proto.MessageType_COMMIT:
		if len(i.commitMessages(view)) >= quorum {
			return "enough commits"
		}
	case proto.MessageType_ROUND_CHANGE:
		msgs := i.verifiedMessages.GetMostRoundChangeMessages()

		// Check for Q(RC)
		if len(msgs) >= int(i.quorumFn(i.backend.ValidatorCount(
			i.state.view.Height))) {
			return "round change"
		}

		// Check for F+1
		if len(msgs) > 0 {
			suggestedRound := msgs[0].Round
			if i.state.view.Round < suggestedRound && len(msgs) > 1000+1 {
				//	TODO: 1000 + 1 -> f(n) + 1
				//		move to new round
				return "round hop"
			}
		}

	}

	return "no event"
}

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
			if !i.isValidMessage(message) {
				// Message is invalid, log it
				i.log.Debug("received invalid message")

				continue
			}

			// Since the message is valid, add it to the verified stack
			i.verifiedMessages.AddMessage(message)

			// Check if any conditions can be met based on the message type
			if event := i.eventPossible(message.Type); event != "no event" {
				i.eventCh <- "something"
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
					if !i.isValidMessage(prepareMessage) {
						// Message is invalid, log it
						i.log.Debug("received invalid message")
					}

					// Since the message is valid, add it to the verified stack
					i.verifiedMessages.AddMessage(prepareMessage)

					// Check if any conditions can be met based on the message type
					if event := i.eventPossible(prepareMessage.Type); event != "no event" {
						i.eventCh <- "something"

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
					if !i.isValidMessage(commitMessage) {
						// Message is invalid, log it
						i.log.Debug("received invalid message")
					}

					// Since the message is valid, add it to the verified stack
					i.verifiedMessages.AddMessage(commitMessage)

					// Check if any conditions can be met based on the message type
					if event := i.eventPossible(commitMessage.Type); event != "no event" {
						i.eventCh <- "something"

						// TODO check if this is valid
						return
					}
				}

			}

		}
	}
}
