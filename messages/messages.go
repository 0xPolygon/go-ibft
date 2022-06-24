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

	// message maps for different message types
	preprepareMessages,
	prepareMessages,
	commitMessages,
	roundChangeMessages heightMessageMap
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

// GetPrePrepareMessage returns a PREPREPARE message, if any
func (ms *Messages) GetPrePrepareMessage(view *proto.View) *PrePrepareMessage {
	ms.Lock()
	defer ms.Unlock()

	if messages := ms.getProtoMessages(view, proto.MessageType_PREPREPARE); messages != nil {
		for _, message := range messages {
			return toPrePrepareFromProto(message)
		}
	}

	return nil
}

// GetPrepareMessages returns all PREPARE messages, if any
func (ms *Messages) GetPrepareMessages(view *proto.View) []*PrepareMessage {
	ms.Lock()
	defer ms.Unlock()

	prepareMessages := make([]*PrepareMessage, 0)
	if messages := ms.getProtoMessages(view, proto.MessageType_PREPARE); messages != nil {
		for _, message := range messages {
			prepareMessages = append(prepareMessages, toPrepareFromProto(message))
		}
	}

	return prepareMessages
}

// GetCommitMessages returns all COMMIT messages, if any
func (ms *Messages) GetCommitMessages(view *proto.View) []*CommitMessage {
	ms.Lock()
	defer ms.Unlock()

	commitMessages := make([]*CommitMessage, 0)
	if messages := ms.getProtoMessages(view, proto.MessageType_COMMIT); messages != nil {
		for _, message := range messages {
			commitMessages = append(commitMessages, toCommitFromProto(message))

		}
	}

	return commitMessages
}

// GetRoundChangeMessages returns all ROUND_CHANGE message, if any
func (ms *Messages) GetRoundChangeMessages(view *proto.View) []*RoundChangeMessage {
	ms.Lock()
	defer ms.Unlock()

	roundChangeMessages := make([]*RoundChangeMessage, 0)
	if messages := ms.getProtoMessages(view, proto.MessageType_ROUND_CHANGE); messages != nil {
		for _, message := range messages {
			roundChangeMessages = append(roundChangeMessages, toRoundChangeFromProto(message))
		}
	}

	return roundChangeMessages
}

// GetMostRoundChangeMessages returns an array of most round change messages
// gathered out of any round
func (ms *Messages) GetMostRoundChangeMessages() []*RoundChangeMessage {
	ms.Lock()
	defer ms.Unlock()

	type maxData struct {
		maxRound        uint64
		maxHeight       uint64
		maxMessagesSize int

		found bool
	}

	var (
		data = maxData{
			maxRound:        0,
			maxHeight:       0,
			maxMessagesSize: 0,
			found:           false,
		}

		heightMsgMap = ms.getMessageMap(proto.MessageType_ROUND_CHANGE)
	)

	// Find the view with the max round change messages
	for heightIndex, roundMessageMap := range heightMsgMap {
		for roundIndex, roundMessages := range roundMessageMap {
			if len(roundMessages) > data.maxMessagesSize || !data.found {
				data.maxRound = roundIndex
				data.maxHeight = heightIndex

				data.maxMessagesSize = len(roundMessages)
				data.found = true
			}
		}
	}

	roundChangeMessages := make([]*RoundChangeMessage, 0)
	if data.found {
		if messages := ms.getProtoMessages(&proto.View{
			Height: data.maxHeight,
			Round:  data.maxRound,
		}, proto.MessageType_ROUND_CHANGE); messages != nil {
			for _, message := range messages {
				roundChangeMessages = append(roundChangeMessages, toRoundChangeFromProto(message))
			}
		}
	}

	return roundChangeMessages
}
