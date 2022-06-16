package messages

import (
	"github.com/Trapesys/go-ibft/proto"
	"sync"
)

// heightMessageMap maps the height number -> round message map
type heightMessageMap map[uint64]*roundMessageMap

// roundMessageMap maps the round number -> messages
type roundMessageMap map[uint64]*protoMessages

// protoMessages is the set of messages that circulate.
// It contains a mapping between the sender and their messages to avoid duplicates
type protoMessages map[string]*proto.Message

// Messages contains the relevant messages for each view (height, round)
type Messages struct {
	// used for thread safety
	sync.RWMutex

	// message maps for different message types
	preprepareMessageMap  *heightMessageMap
	prepareMessageMap     *heightMessageMap
	commitMessageMap      *heightMessageMap
	roundChangeMessageMap *heightMessageMap
}

func (h *heightMessageMap) getViewMessages(view *proto.View) *protoMessages {
	// Check if the height is present
	roundMessages, exists := (*h)[view.Height]
	if !exists {
		roundMessages = &roundMessageMap{
			view.Round: &protoMessages{},
		}

		(*h)[view.Height] = roundMessages
	}

	// Check if the round is present
	messages, exists := (*roundMessages)[view.Round]
	if !exists {
		messages = &protoMessages{}

		(*roundMessages)[view.Round] = messages
	}

	return messages
}

// NewMessages returns a new Messages wrapper
func NewMessages() *Messages {
	return &Messages{
		preprepareMessageMap:  &heightMessageMap{},
		prepareMessageMap:     &heightMessageMap{},
		commitMessageMap:      &heightMessageMap{},
		roundChangeMessageMap: &heightMessageMap{},
	}
}

// getMessageMap fetches the corresponding message map by type
func (ms *Messages) getMessageMap(messageType proto.MessageType) *heightMessageMap {
	switch messageType {
	case proto.MessageType_PREPREPARE:
		return ms.preprepareMessageMap
	case proto.MessageType_PREPARE:
		return ms.prepareMessageMap
	case proto.MessageType_COMMIT:
		return ms.commitMessageMap
	case proto.MessageType_ROUND_CHANGE:
		return ms.roundChangeMessageMap
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
	(*messages)[string(message.From)] = message
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
	roundMsgMap, found := (*heightMsgMap)[view.Height]
	if !found {
		return 0
	}

	// Check if the messages array is present
	messages, found := (*roundMsgMap)[view.Round]
	if !found {
		return 0
	}

	return len(*messages)
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
		delete(*messageMap, view.Height)
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

		heightMap, exists := (*typeMap)[view.Height]
		if exists {
			delete(*heightMap, view.Round)
		}
	}
}
