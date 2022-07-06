package messages

import (
	"sync"

	"github.com/Trapesys/go-ibft/messages/proto"
)

// heightMessageMap maps the height number -> round message map
type heightMessageMap map[uint64]roundMessageMap

// roundMessageMap maps the round number -> messages
type roundMessageMap map[uint64]protoMessages

// protoMessages is the set of messages that circulate.
// It contains a mapping between the sender and their messages to avoid duplicates
type protoMessages map[string]*proto.Message

// Messages contains the relevant messages for each view (height, round)
type Messages struct {
	sync.RWMutex

	eventManager *eventManager

	// message maps for different message types
	preprepareMessages,
	prepareMessages,
	commitMessages,
	roundChangeMessages heightMessageMap
}

// Subscribe creates a new message type subscription
func (ms *Messages) Subscribe(details Subscription) *SubscribeResult {
	// Create the subscription
	subscription := ms.eventManager.subscribe(details)

	// Check if any condition is already met
	numMessages := ms.numMessages(details.View, details.MessageType)
	if numMessages >= details.NumMessages {
		// Conditions are already met, alert the event manager
		ms.eventManager.signalEvent(details.MessageType, details.View, numMessages)
	}

	return subscription
}

// Unsubscribe cancels a message type subscription
func (ms *Messages) Unsubscribe(id SubscriptionID) {
	ms.eventManager.cancelSubscription(id)
}

// NewMessages returns a new Messages wrapper
func NewMessages() *Messages {
	return &Messages{
		preprepareMessages:  make(heightMessageMap),
		prepareMessages:     make(heightMessageMap),
		commitMessages:      make(heightMessageMap),
		roundChangeMessages: make(heightMessageMap),

		eventManager: newEventManager(),
	}
}

// AddMessage adds a new message to the message queue
func (ms *Messages) AddMessage(message *proto.Message) {
	ms.Lock()
	defer ms.Unlock()

	// Get the corresponding height map
	heightMsgMap := ms.getMessageMap(message.Type)

	// Append the message to the appropriate queue
	messages := heightMsgMap.getViewMessages(message.View)
	messages[string(message.From)] = message

	ms.eventManager.signalEvent(
		message.Type,
		message.View, // TODO ptr?
		len(messages),
	)
}

func (ms *Messages) Close() {
	ms.eventManager.close()
}

// getMessageMap fetches the corresponding message map by type
func (ms *Messages) getMessageMap(messageType proto.MessageType) heightMessageMap {
	switch messageType {
	case proto.MessageType_PREPREPARE:
		return ms.preprepareMessages
	case proto.MessageType_PREPARE:
		return ms.prepareMessages
	case proto.MessageType_COMMIT:
		return ms.commitMessages
	case proto.MessageType_ROUND_CHANGE:
		return ms.roundChangeMessages
	}

	return nil
}

// getViewMessages fetches the message queue for the specified view (height + round).
// It will initialize a new message array if it's not found
func (m heightMessageMap) getViewMessages(view *proto.View) protoMessages {
	var (
		height = view.Height
		round  = view.Round
	)

	// Check if the height is present
	roundMessages, exists := m[height]
	if !exists {
		roundMessages = roundMessageMap{}

		m[height] = roundMessages
	}

	// Check if the round is present
	messages, exists := roundMessages[round]
	if !exists {
		messages = protoMessages{}

		roundMessages[round] = messages
	}

	return messages
}

// numMessages returns the number of messages received for the specific type
func (ms *Messages) numMessages(
	view *proto.View,
	messageType proto.MessageType,
) int {
	ms.RLock()
	defer ms.RUnlock()

	heightMsgMap := ms.getMessageMap(messageType)

	// Check if the round map is present
	roundMsgMap, found := heightMsgMap[view.Height]
	if !found {
		return 0
	}

	// Check if the messages array is present
	messages, found := roundMsgMap[view.Round]
	if !found {
		return 0
	}

	return len(messages)
}

// PruneByHeight prunes out all old messages from the message queues
// by the specified height in the view
func (ms *Messages) PruneByHeight(view *proto.View) {
	ms.Lock()
	defer ms.Unlock()

	possibleMaps := []proto.MessageType{
		proto.MessageType_PREPREPARE,
		proto.MessageType_PREPARE,
		proto.MessageType_COMMIT,
		proto.MessageType_ROUND_CHANGE,
	}

	// Prune out the views from all possible message types
	for _, messageType := range possibleMaps {
		messageMap := ms.getMessageMap(messageType)

		// Delete all height maps up until and including the specified
		// view height
		for height := uint64(0); height <= view.Height; height++ {
			delete(messageMap, height)
		}
	}
}

// getProtoMessages fetches the underlying proto messages for the specified view
// and message type
func (ms *Messages) getProtoMessages(
	view *proto.View,
	messageType proto.MessageType,
) protoMessages {
	heightMsgMap := ms.getMessageMap(messageType)

	// Check if the round map is present
	roundMsgMap, found := heightMsgMap[view.Height]
	if !found {
		return nil
	}

	return roundMsgMap[view.Round]
}

// GetValidMessages fetches all messages of a specific type for the specified view,
// that pass the validity check; invalid messages are pruned out
func (ms *Messages) GetValidMessages(
	view *proto.View,
	messageType proto.MessageType,
	isValid func(message *proto.Message) bool,
) []*proto.Message {
	ms.Lock()
	defer ms.Unlock()

	result := make([]*proto.Message, 0)

	invalidMessages := make([]string, 0)
	messages := ms.getProtoMessages(view, messageType)
	if messages != nil {
		for key, message := range messages {
			if !isValid(message) {
				invalidMessages = append(invalidMessages, key)

				continue
			}

			result = append(result, message)
		}
	}

	// Prune out invalid messages
	// TODO @dusan there shouldn't be any
	// danger in doing this?
	for _, key := range invalidMessages {
		delete(messages, key)
	}

	return result
}

// GetMostRoundChangeMessages fetches most round change messages
// for the minimum round and above
func (ms *Messages) GetMostRoundChangeMessages(minRound, height uint64) []*proto.Message {
	ms.RLock()
	defer ms.RUnlock()

	roundMessageMap := ms.getMessageMap(proto.MessageType_ROUND_CHANGE)[height]

	var (
		bestRound              = uint64(0)
		bestRoundMessagesCount = 0
	)

	for round, msgs := range roundMessageMap {
		if round < minRound {
			continue
		}

		if len(msgs) > bestRoundMessagesCount {
			bestRound = round
		}
	}

	if bestRound == 0 {
		//	no messages found
		return nil
	}

	messages := make([]*proto.Message, 0, bestRoundMessagesCount)
	for _, msg := range roundMessageMap[bestRound] {
		messages = append(messages, msg)
	}

	return messages
}

// ExtractCommittedSeals extracts the committed seals from the passed in messages
func ExtractCommittedSeals(commitMessages []*proto.Message) [][]byte {
	committedSeals := make([][]byte, len(commitMessages))

	for index, commitMessage := range commitMessages {
		committedSeals[index] = ExtractCommittedSeal(commitMessage)
	}

	return committedSeals
}

// ExtractCommittedSeal extracts the committed seal from the passed in message
func ExtractCommittedSeal(commitMessage *proto.Message) []byte {
	return commitMessage.Payload.(*proto.Message_CommitData).CommitData.CommittedSeal
}

// ExtractCommitHash extracts the commit proposal hash from the passed in message
func ExtractCommitHash(commitMessage *proto.Message) []byte {
	return commitMessage.Payload.(*proto.Message_CommitData).CommitData.ProposalHash
}

// ExtractProposal extracts the proposal from the passed in message
func ExtractProposal(proposalMessage *proto.Message) []byte {
	return proposalMessage.Payload.(*proto.Message_PreprepareData).PreprepareData.Proposal
}

// ExtractPrepareHash extracts the prepare proposal hash from the passed in message
func ExtractPrepareHash(prepareMessage *proto.Message) []byte {
	return prepareMessage.Payload.(*proto.Message_PrepareData).PrepareData.ProposalHash
}
