package messages

import (
	"sync"

	"github.com/Trapesys/go-ibft/proto"
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
	// used for thread safety
	sync.RWMutex

	// message maps for different message types
	preprepareMessages,
	prepareMessages,
	commitMessages,
	roundChangeMessages heightMessageMap
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
		roundMessages = roundMessageMap{round: protoMessages{}}

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

// NewMessages returns a new Messages wrapper
func NewMessages() *Messages {
	return &Messages{
		preprepareMessages:  make(heightMessageMap),
		prepareMessages:     make(heightMessageMap),
		commitMessages:      make(heightMessageMap),
		roundChangeMessages: make(heightMessageMap),
	}
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

// AddMessage adds a new message to the message queue
func (ms *Messages) AddMessage(message *proto.Message) {
	ms.Lock()
	defer ms.Unlock()

	// Get the corresponding height map
	heightMsgMap := ms.getMessageMap(message.Type)

	// Append the message to the appropriate queue
	messages := heightMsgMap.getViewMessages(message.View)
	messages[string(message.From)] = message
}

// NumMessages returns the number of messages received for the specific type
func (ms *Messages) NumMessages(
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

// PruneByRound prunes out all old messages from the message queues
// by the specified round in the view
func (ms *Messages) PruneByRound(view *proto.View) {
	ms.Lock()
	defer ms.Unlock()

	possibleMaps := []proto.MessageType{
		proto.MessageType_PREPREPARE,
		proto.MessageType_PREPARE,
		proto.MessageType_COMMIT,
		proto.MessageType_ROUND_CHANGE,
	}

	// Prune out the rounds from all possible message types
	for _, messageType := range possibleMaps {
		typeMap := ms.getMessageMap(messageType)

		heightMap, exists := typeMap[view.Height]
		if exists {
			// Delete all round maps up until and including the specified
			// view round
			for round := uint64(0); round <= view.Round; round++ {
				delete(heightMap, round)
			}
		}
	}
}
