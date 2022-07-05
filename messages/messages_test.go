package messages

import (
	"github.com/Trapesys/go-ibft/messages/proto"
	"github.com/stretchr/testify/assert"
	"strconv"
	"testing"
	"time"
)

// generateRandomMessages generates random messages for the
func generateRandomMessages(
	count int,
	view *proto.View,
	messageTypes ...proto.MessageType,
) []*proto.Message {
	messages := make([]*proto.Message, 0)

	for index := 0; index < count; index++ {
		for _, messageType := range messageTypes {
			message := &proto.Message{
				From: []byte(strconv.Itoa(index)),
				View: view,
				Type: messageType,
			}

			switch messageType {
			case proto.MessageType_PREPREPARE:
				message.Payload = &proto.Message_PreprepareData{
					PreprepareData: &proto.PrePrepareMessage{
						Proposal: nil,
					},
				}
			case proto.MessageType_PREPARE:
				message.Payload = &proto.Message_PrepareData{
					PrepareData: &proto.PrepareMessage{
						ProposalHash: nil,
					},
				}
			case proto.MessageType_COMMIT:
				message.Payload = &proto.Message_CommitData{
					CommitData: &proto.CommitMessage{
						ProposalHash:  nil,
						CommittedSeal: nil,
					},
				}
			}

			messages = append(messages, message)
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
	defer messages.Close()

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

// TestMessages_AddDuplicates tests that no duplicates
// can be added to the height -> round -> message queue,
// meaning a sender cannot fill the message queue with duplicate messages
// of the same view for the same message type
func TestMessages_AddDuplicates(t *testing.T) {
	t.Parallel()

	numMessages := 5
	commonSender := strconv.Itoa(1)
	commonType := proto.MessageType_PREPARE
	initialView := &proto.View{
		Height: 1,
		Round:  1,
	}

	messages := NewMessages()
	defer messages.Close()

	// Append random message types
	randomMessages := generateRandomMessages(
		numMessages,
		initialView,
		commonType,
	)

	for _, message := range randomMessages {
		message.From = []byte(commonSender)
		messages.AddMessage(message)
	}

	// Check that only 1 message has been added
	assert.Equal(t, 1, messages.NumMessages(initialView, commonType))
}

// TestMessages_Prune tests if pruning of certain messages works
func TestMessages_Prune(t *testing.T) {
	t.Parallel()

	numMessages := 5
	messageType := proto.MessageType_PREPARE
	messages := NewMessages()
	defer messages.Close()

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

// TestMessages_GetMessage makes sure
// that messages are fetched correctly for the
// corresponding message type
func TestMessages_GetMessage(t *testing.T) {
	t.Parallel()

	var (
		defaultView = &proto.View{
			Height: 1,
			Round:  0,
		}
		numMessages = 5
	)

	testTable := []struct {
		name        string
		messageType proto.MessageType
	}{
		{
			"Fetch PREPREPAREs",
			proto.MessageType_PREPREPARE,
		},
		{
			"Fetch PREPAREs",
			proto.MessageType_PREPARE,
		},
		{
			"Fetch COMMITs",
			proto.MessageType_COMMIT,
		},
		{
			"Fetch ROUND_CHANGEs",
			proto.MessageType_ROUND_CHANGE,
		},
	}

	for _, testCase := range testTable {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			// Add the initial message set
			messages := NewMessages()
			defer messages.Close()

			// Generate random messages
			randomMessages := generateRandomMessages(
				numMessages,
				defaultView,
				testCase.messageType,
			)

			// Add the messages to the corresponding queue
			for _, message := range randomMessages {
				messages.AddMessage(message)
			}

			// Make sure the messages are there
			assert.Equal(
				t,
				numMessages,
				messages.NumMessages(defaultView, testCase.messageType),
			)

			// Start fetching messages and making sure they're not cleared
			switch testCase.messageType {
			case proto.MessageType_PREPREPARE:
				messages.GetMessages(defaultView, proto.MessageType_PREPREPARE)
			case proto.MessageType_PREPARE:
				messages.GetMessages(defaultView, proto.MessageType_PREPARE)
			case proto.MessageType_COMMIT:
				messages.GetMessages(defaultView, proto.MessageType_COMMIT)
			case proto.MessageType_ROUND_CHANGE:
				messages.GetMessages(defaultView, proto.MessageType_ROUND_CHANGE)
			}

			assert.Equal(
				t,
				numMessages,
				messages.NumMessages(defaultView, testCase.messageType),
			)
		})
	}
}

// TestMessages_GetMostRoundChangeMessages makes sure
// the round messages for the round with the most round change
// messages are fetched
func TestMessages_GetMostRoundChangeMessages(t *testing.T) {
	t.Parallel()

	messages := NewMessages()
	defer messages.Close()

	mostMessageCount := 3
	mostMessagesRound := uint64(2)

	// Generate round messages
	randomMessages := map[uint64][]*proto.Message{
		0: generateRandomMessages(mostMessageCount-2, &proto.View{
			Height: 0,
			Round:  0,
		}, proto.MessageType_ROUND_CHANGE),
		1: generateRandomMessages(mostMessageCount-1, &proto.View{
			Height: 0,
			Round:  1,
		}, proto.MessageType_ROUND_CHANGE),
		mostMessagesRound: generateRandomMessages(mostMessageCount, &proto.View{
			Height: 0,
			Round:  mostMessagesRound,
		}, proto.MessageType_ROUND_CHANGE),
	}

	// Add the messages
	for _, roundMessages := range randomMessages {
		for _, message := range roundMessages {
			messages.AddMessage(message)
		}
	}

	roundChangeMessages := messages.GetMostRoundChangeMessages(0, 0)

	if len(roundChangeMessages) != mostMessageCount {
		t.Fatalf("Invalid number of round change messages, %d", len(roundChangeMessages))
	}

	assert.Equal(t, mostMessagesRound, roundChangeMessages[0].View.Round)
}

// TestMessages_EventManager checks that the event manager
// behaves correctly when new messages appear
func TestMessages_EventManager(t *testing.T) {
	t.Parallel()

	messages := NewMessages()
	defer messages.Close()

	numMessages := 10
	messageType := proto.MessageType_PREPARE
	baseView := &proto.View{
		Height: 0,
		Round:  0,
	}

	// Create the subscription
	subscription := messages.Subscribe(SubscriptionDetails{
		MessageType: messageType,
		View:        baseView,
		NumMessages: numMessages,
	})

	defer messages.Unsubscribe(subscription.GetID())

	// Push random messages
	randomMessages := generateRandomMessages(numMessages, baseView, messageType)
	for _, message := range randomMessages {
		messages.AddMessage(message)
	}

	// Wait for the subscription event to happen
	select {
	case <-subscription.GetCh():
	case <-time.After(5 * time.Second):
	}

	// Make sure the number of messages is actually accurate
	assert.Equal(t, numMessages, messages.NumMessages(baseView, messageType))
}
