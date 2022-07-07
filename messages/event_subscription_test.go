package messages

import (
	"github.com/Trapesys/go-ibft/messages/proto"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestEventSubscription_EventSupported(t *testing.T) {
	t.Parallel()

	supportedDetails := Subscription{
		MessageType: proto.MessageType_PREPARE,
		View: &proto.View{
			Height: 0,
			Round:  0,
		},
		NumMessages: 10,
	}

	subscription := &eventSubscription{
		details:  supportedDetails,
		outputCh: make(chan struct{}, 1),
		notifyCh: make(chan struct{}, 1),
		doneCh:   make(chan struct{}),
	}

	t.Cleanup(func() {
		subscription.close()
	})

	type signalDetails struct {
		messageType   proto.MessageType
		view          *proto.View
		totalMessages int
	}

	testTable := []struct {
		name      string
		details   []signalDetails
		supported bool
	}{
		{
			"Supported events processed",
			[]signalDetails{
				{
					supportedDetails.MessageType,
					supportedDetails.View,
					supportedDetails.NumMessages,
				},
			},
			true,
		},
		{
			"Unsupported events not processed",
			[]signalDetails{
				{
					messageType:   supportedDetails.MessageType,
					view:          supportedDetails.View,
					totalMessages: 9,
				},
				{
					messageType:   proto.MessageType_COMMIT,
					view:          supportedDetails.View,
					totalMessages: supportedDetails.NumMessages,
				},
				{
					messageType: supportedDetails.MessageType,
					view: &proto.View{
						Height: supportedDetails.View.Height,
						Round:  supportedDetails.View.Round + 1,
					},
					totalMessages: supportedDetails.NumMessages,
				},
				{
					messageType: supportedDetails.MessageType,
					view: &proto.View{
						Height: supportedDetails.View.Height + 1,
						Round:  supportedDetails.View.Round,
					},
					totalMessages: supportedDetails.NumMessages,
				},
			},
			false,
		},
	}

	for _, testCase := range testTable {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			for _, details := range testCase.details {
				assert.Equal(
					t,
					testCase.supported,
					subscription.eventSupported(
						details.messageType,
						details.view,
						details.totalMessages,
					),
				)
			}
		})
	}
}
