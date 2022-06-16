package messages

import (
	"github.com/Trapesys/go-ibft/proto"
	"github.com/stretchr/testify/assert"
	"testing"
)

// generateRandomMessages generates random messages for the
func generateRandomMessages(
	count int,
	view *proto.View,
	messageTypes ...proto.MessageType) []*proto.Message {
	messages := make([]*proto.Message, 0)

	for index := 0; index < count; index++ {
		for _, messageType := range messageTypes {
			messages = append(messages, &proto.Message{
				View: view,
				Type: messageType,
			})
		}
	}

	return messages
}

// TestMessages_AddMessage tests if the message addition
// of different types works
func TestMessages_AddMessage(t *testing.T) {
	t.Parallel()

	numMessages := 5
	initialView := &proto.View{
		Height: 1,
		Round:  1,
	}

	messages := NewMessages()

	// Append random message types
	randomMessages := generateRandomMessages(
		numMessages,
		initialView,
		proto.MessageType_PREPARE,
		proto.MessageType_COMMIT,
		proto.MessageType_ROUND_CHANGE,
	)

	for _, message := range randomMessages {
		messages.AddMessage(message)
	}

	// Make sure that the messages are present
	assert.Equal(t, numMessages, messages.NumMessages(initialView, proto.MessageType_PREPARE))
	assert.Equal(t, numMessages, messages.NumMessages(initialView, proto.MessageType_COMMIT))
	assert.Equal(t, numMessages, messages.NumMessages(initialView, proto.MessageType_ROUND_CHANGE))
}

// TestMessages_Prune tests if pruning of certain messages works
func TestMessages_Prune(t *testing.T) {
	t.Parallel()

	numMessages := 5
	messageType := proto.MessageType_PREPARE
	messages := NewMessages()

	views := make([]*proto.View, 0)
	for index := uint64(1); index <= 3; index++ {
		views = append(views, &proto.View{
			Height: 1,
			Round:  index,
		})
	}

	// Append random message types
	randomMessages := make([]*proto.Message, 0)
	for _, view := range views {
		randomMessages = append(
			randomMessages,
			generateRandomMessages(
				numMessages,
				view,
				messageType,
			)...,
		)
	}

	for _, message := range randomMessages {
		messages.AddMessage(message)
	}

	// Prune out the messages from this view
	messages.PruneByRound(views[0])

	// Make sure the round 1 messages are pruned out
	assert.Equal(t, 0, messages.NumMessages(views[0], messageType))

	// Make sure the round 2 messages are still present
	assert.Equal(t, numMessages, messages.NumMessages(views[1], messageType))

	// Make sure the round 3 messages are still present
	assert.Equal(t, numMessages, messages.NumMessages(views[2], messageType))

	// Prune out the messages from this view
	messages.PruneByHeight(views[1])

	// Make sure the round 2 messages are pruned out
	assert.Equal(t, 0, messages.NumMessages(views[1], messageType))

	// Make sure the round 3 messages are pruned out
	assert.Equal(t, 0, messages.NumMessages(views[2], messageType))
}
