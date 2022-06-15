package messages

import (
	"github.com/Trapesys/go-ibft/proto"
	"sync"
)

// typeMessageMap maps the message type -> height message map
type typeMessageMap map[proto.MessageType]*heightMessageMap

// heightMessageMap maps the height number -> round message map
type heightMessageMap map[uint64]*roundMessageMap

// roundMessageMap maps the round number -> messages
type roundMessageMap map[uint64]*messageHeap

// Messages contains the relevant messages for each view (height, round)
type Messages struct {
	// used for thread safety
	sync.RWMutex

	// messageMap contains the relevant view -> message queues mapping
	messageMap *typeMessageMap
}

func (h *heightMessageMap) getViewMessageHeap(view *proto.View) *messageHeap {
	// Check if the height is present
	roundMessages, exists := (*h)[view.Height]
	if !exists {
		roundMessages = &roundMessageMap{
			view.Round: &messageHeap{},
		}

		(*h)[view.Height] = roundMessages
	}

	// Check if the round is present
	heap, exists := (*roundMessages)[view.Round]
	if !exists {
		heap = &messageHeap{}

		(*roundMessages)[view.Round] = heap
	}

	return heap
}

// NewMessages returns a new Messages wrapper
func NewMessages() *Messages {
	return &Messages{
		messageMap: &typeMessageMap{
			proto.MessageType_PREPREPARE:   &heightMessageMap{},
			proto.MessageType_PREPARE:      &heightMessageMap{},
			proto.MessageType_COMMIT:       &heightMessageMap{},
			proto.MessageType_ROUND_CHANGE: &heightMessageMap{},
		},
	}
}

// AddMessage adds a new message to the message queue
func (ms *Messages) AddMessage(message *proto.Message) {
	ms.Lock()
	defer ms.Unlock()

	// Get the corresponding height map
	heightMsgMap := (*ms.messageMap)[message.Type]

	// Append the message to the appropriate queue
	heap := heightMsgMap.getViewMessageHeap(message.View)
	heap.Push(message)
}

// NumMessages returns the number of messages received for the specific type
func (ms *Messages) NumMessages(
	view *proto.View,
	messageType proto.MessageType,
) int {
	ms.RLock()
	defer ms.RUnlock()

	// Check if the height map is present
	heightMsgMap, found := (*ms.messageMap)[messageType]
	if !found {
		return 0
	}

	// Check if the round map is present
	roundMsgHeap, found := (*heightMsgMap)[view.Height]
	if !found {
		return 0
	}

	// Check if the heap is present
	heap, found := (*roundMsgHeap)[view.Round]
	if !found {
		return 0
	}

	return heap.Len()
}

// Prune prunes out all old messages from the message queues
func (ms *Messages) Prune(view *proto.View) {
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
		messageMap := (*ms.messageMap)[messageType]
		delete(*messageMap, view.Height)
	}
}
