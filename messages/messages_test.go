package messages

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"

	"github.com/0xPolygon/go-ibft/messages/proto"
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

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
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
	assert.Equal(t, numMessages, messages.numMessages(initialView, proto.MessageType_PREPARE))
	assert.Equal(t, numMessages, messages.numMessages(initialView, proto.MessageType_COMMIT))
	assert.Equal(t, numMessages, messages.numMessages(initialView, proto.MessageType_ROUND_CHANGE))
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
	assert.Equal(t, 1, messages.numMessages(initialView, commonType))
}

// TestMessages_Prune tests if pruning of certain messages works
func TestMessages_Prune(t *testing.T) {
	t.Parallel()

	var (
		numMessages = 5
		messageType = proto.MessageType_PREPARE

		height uint64 = 2
	)

	messages := NewMessages()

	t.Cleanup(func() {
		messages.Close()
	})

	views := []*proto.View{
		{
			Height: height - 1,
			Round:  1,
		},
		{
			Height: height,
			Round:  2,
		},
		{
			Height: height + 1,
			Round:  3,
		},
	}

	// expected number of message for each view after pruning
	expectedNumMessages := []int{
		0,
		numMessages,
		numMessages,
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
	messages.PruneByHeight(height)

	// check numbers of messages
	for idx, expected := range expectedNumMessages {
		assert.Equal(
			t,
			expected,
			messages.numMessages(
				views[idx],
				messageType,
			),
		)
	}
}

// TestMessages_GetValidMessagesMessage_InvalidMessages makes sure
// that messages are fetched correctly for the
// corresponding message type
func TestMessages_GetValidMessagesMessage(t *testing.T) {
	t.Parallel()

	var (
		defaultView = &proto.View{
			Height: 1,
			Round:  0,
		}

		numMessages      = 10
		numValidMessages = 5
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

	newIsValid := func(numValidMessages int) func(_ *proto.Message) bool {
		calls := 0

		return func(_ *proto.Message) bool {
			calls++

			return calls <= numValidMessages
		}
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
				messages.numMessages(defaultView, testCase.messageType),
			)

			// Start fetching messages and making sure they're not cleared
			validMessages := messages.GetValidMessages(
				defaultView,
				testCase.messageType,
				newIsValid(numValidMessages),
			)

			// make sure only valid messages are returned
			assert.Len(
				t,
				validMessages,
				numValidMessages,
			)

			// make sure invalid messages are pruned
			assert.Equal(
				t,
				numMessages-numValidMessages,
				messages.numMessages(defaultView, testCase.messageType),
			)
		})
	}
}

// TestMessages_GetMostRoundChangeMessages makes sure
// the round messages for the round with the most round change
// messages are fetched
func TestMessages_GetMostRoundChangeMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		messages      [][]*proto.Message
		minRound      uint64
		height        uint64
		expectedNum   int
		expectedRound uint64
	}{
		{
			name: "should return nil if not found",
			messages: [][]*proto.Message{
				generateRandomMessages(3, &proto.View{
					Height: 0,
					Round:  1, // smaller than minRound
				}, proto.MessageType_ROUND_CHANGE),
			},
			minRound:    2,
			height:      0,
			expectedNum: 0,
		},
		{
			name: "should return round change messages if messages' round is greater than/equal to minRound",
			messages: [][]*proto.Message{
				generateRandomMessages(1, &proto.View{
					Height: 0,
					Round:  2,
				}, proto.MessageType_ROUND_CHANGE),
			},
			minRound:      1,
			height:        0,
			expectedNum:   1,
			expectedRound: 2,
		},
		{
			name: "should return most round change messages (the round is equals to minRound)",
			messages: [][]*proto.Message{
				generateRandomMessages(1, &proto.View{
					Height: 0,
					Round:  4,
				}, proto.MessageType_ROUND_CHANGE),
				generateRandomMessages(2, &proto.View{
					Height: 0,
					Round:  2,
				}, proto.MessageType_ROUND_CHANGE),
			},
			minRound:      2,
			height:        0,
			expectedNum:   2,
			expectedRound: 2,
		},
		{
			name: "should return most round change messages (the round is bigger than minRound)",
			messages: [][]*proto.Message{
				generateRandomMessages(3, &proto.View{
					Height: 0,
					Round:  1,
				}, proto.MessageType_ROUND_CHANGE),
				generateRandomMessages(2, &proto.View{
					Height: 0,
					Round:  3,
				}, proto.MessageType_ROUND_CHANGE),
				generateRandomMessages(1, &proto.View{
					Height: 0,
					Round:  4,
				}, proto.MessageType_ROUND_CHANGE),
			},
			minRound:      2,
			height:        0,
			expectedNum:   2,
			expectedRound: 3,
		},
		{
			name: "should return the first of most round change messages",
			messages: [][]*proto.Message{
				generateRandomMessages(3, &proto.View{
					Height: 0,
					Round:  1,
				}, proto.MessageType_ROUND_CHANGE),
				generateRandomMessages(2, &proto.View{
					Height: 0,
					Round:  4,
				}, proto.MessageType_ROUND_CHANGE),
				generateRandomMessages(2, &proto.View{
					Height: 0,
					Round:  3,
				}, proto.MessageType_ROUND_CHANGE),
			},
			minRound:      2,
			height:        0,
			expectedNum:   2,
			expectedRound: 4,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			messages := NewMessages()
			defer messages.Close()

			// Add the messages
			for _, roundMessages := range test.messages {
				for _, message := range roundMessages {
					messages.AddMessage(message)
				}
			}

			roundChangeMessages := messages.GetMostRoundChangeMessages(test.minRound, test.height)

			if test.expectedNum == 0 {
				assert.Nil(t, roundChangeMessages, "should be nil but not nil")
			} else {
				assert.Len(t, roundChangeMessages, test.expectedNum, "invalid number of round change messages")
			}

			for _, msg := range roundChangeMessages {
				assert.Equal(t, test.expectedRound, msg.View.Round)
			}
		})
	}
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
		HasQuorumFn: func(_ uint64, messages []*proto.Message, _ proto.MessageType) bool {
			return len(messages) >= numMessages
		},
	})

	defer messages.Unsubscribe(subscription.ID)

	// Push random messages
	randomMessages := generateRandomMessages(numMessages, baseView, messageType)
	for _, message := range randomMessages {
		messages.AddMessage(message)
	}

	// Wait for the subscription event to happen
	select {
	case <-subscription.SubCh:
	case <-time.After(5 * time.Second):
	}

	// Make sure the number of messages is actually accurate
	assert.Equal(t, numMessages, messages.numMessages(baseView, messageType))
}

// TestMessages_Unsubscribe checks Messages calls eventManager.cancelSubscription
// in Unsubscribe method
func TestMessages_Unsubscribe(t *testing.T) {
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
		HasQuorumFn: func(_ uint64, messages []*proto.Message, _ proto.MessageType) bool {
			return len(messages) >= numMessages
		},
	})

	assert.Equal(t, int64(1), messages.eventManager.numSubscriptions)

	messages.Unsubscribe(subscription.ID)

	assert.Equal(t, int64(0), messages.eventManager.numSubscriptions)
}

// TestMessages_Unsubscribe checks Messages calls eventManager.close
// in Close method
func TestMessages_Close(t *testing.T) {
	t.Parallel()

	messages := NewMessages()
	defer messages.Close()

	numMessages := 10
	baseView := &proto.View{
		Height: 0,
		Round:  0,
	}

	// Create 2 subscriptions
	_ = messages.Subscribe(SubscriptionDetails{
		MessageType: proto.MessageType_PREPARE,
		View:        baseView,
		HasQuorumFn: func(_ uint64, messages []*proto.Message, _ proto.MessageType) bool {
			return len(messages) >= numMessages
		},
	})

	_ = messages.Subscribe(SubscriptionDetails{
		MessageType: proto.MessageType_COMMIT,
		View:        baseView,
		HasQuorumFn: func(_ uint64, messages []*proto.Message, _ proto.MessageType) bool {
			return len(messages) >= numMessages
		},
	})

	assert.Equal(t, int64(2), messages.eventManager.numSubscriptions)

	messages.Close()

	assert.Equal(t, int64(0), messages.eventManager.numSubscriptions)
}

func TestMessages_getProtoMessage(t *testing.T) {
	t.Parallel()

	messages := NewMessages()
	defer messages.Close()

	var (
		numMessages = 10
		messageType = proto.MessageType_COMMIT
		view        = &proto.View{
			Height: 0,
			Round:  0,
		}
	)

	// Create the subscription
	subscription := messages.Subscribe(SubscriptionDetails{
		MessageType: messageType,
		View:        view,
		HasQuorumFn: func(_ uint64, messages []*proto.Message, _ proto.MessageType) bool {
			return len(messages) >= numMessages
		},
	})

	defer messages.Unsubscribe(subscription.ID)

	// Push random messages
	generatedMessages := generateRandomMessages(numMessages, view, messageType)
	messageMap := map[string]*proto.Message{}

	for _, message := range generatedMessages {
		messages.AddMessage(message)
		messageMap[string(message.From)] = message
	}

	// Wait for the subscription event to happen
	select {
	case <-subscription.SubCh:
	case <-time.After(5 * time.Second):
	}

	tests := []struct {
		name        string
		view        *proto.View
		messageType proto.MessageType
		expected    protoMessages
	}{
		{
			name:        "should return messages for same view and type",
			view:        view,
			messageType: messageType,
			expected:    messageMap,
		},
		{
			name:        "should return nil for different type",
			view:        view,
			messageType: proto.MessageType_PREPARE,
			expected:    nil,
		},
		{
			name: "should return nil for same type and round but different height",
			view: &proto.View{
				Height: view.Height + 1,
				Round:  view.Round,
			},
			messageType: messageType,
			expected:    nil,
		},
		{
			name: "should return nil for same type and height but different round",
			view: &proto.View{
				Height: view.Height,
				Round:  view.Round + 1,
			},
			messageType: messageType,
			expected:    nil,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(
				t,
				test.expected,
				messages.getProtoMessages(test.view, test.messageType),
			)
		})
	}
}

func TestMessages_numMessages(t *testing.T) {
	t.Parallel()

	messages := NewMessages()
	defer messages.Close()

	var (
		numMessages = 10
		messageType = proto.MessageType_COMMIT
		view        = &proto.View{
			Height: 3,
			Round:  5,
		}
	)

	// Create the subscription
	subscription := messages.Subscribe(SubscriptionDetails{
		MessageType: messageType,
		View:        view,
		HasQuorumFn: func(_ uint64, messages []*proto.Message, _ proto.MessageType) bool {
			return len(messages) >= numMessages
		},
	})

	defer messages.Unsubscribe(subscription.ID)

	// Push random messages
	for _, message := range generateRandomMessages(numMessages, view, messageType) {
		messages.AddMessage(message)
	}

	// Wait for the subscription event to happen
	select {
	case <-subscription.SubCh:
	case <-time.After(5 * time.Second):
	}

	tests := []struct {
		name        string
		view        *proto.View
		messageType proto.MessageType
		expected    int
	}{
		{
			name:        "should return number of messages",
			view:        view,
			messageType: messageType,
			expected:    numMessages,
		},
		{
			name:        "should return zero if message type is different",
			view:        view,
			messageType: proto.MessageType_PREPARE,
			expected:    0,
		},
		{
			name: "should return zero if height is different",
			view: &proto.View{
				Height: 1,
				Round:  view.Round,
			},
			messageType: messageType,
			expected:    0,
		},
		{
			name: "should return zero if round is different",
			view: &proto.View{
				Height: view.Height,
				Round:  1,
			},
			messageType: messageType,
			expected:    0,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, messages.numMessages(test.view, test.messageType))
		})
	}
}
