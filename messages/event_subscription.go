package messages

import (
	"github.com/Trapesys/go-ibft/messages/proto"
)

type SubscriptionID int32

type eventSubscription struct {
	// details contains the details of the event subscription
	details Subscription

	// outputCh is the update channel for the subscriber
	outputCh chan struct{}

	// doneCh is the channel for handling stop signals
	doneCh chan struct{}

	// notifyCh is the channel for receiving event requests
	notifyCh chan struct{}
}

// close stops the event subscription
func (es *eventSubscription) close() {
	close(es.doneCh)
	//	TODO: fix panic
	//close(es.outputCh)
}

// runLoop is the main loop that listens for notifications and handles the event / close signals
func (es *eventSubscription) runLoop() {
	for {
		select {
		case <-es.doneCh: // Break if a close signal has been received
			return
		case <-es.notifyCh: // Listen for new events to appear
			select {
			case <-es.doneCh: // Break if a close signal has been received
				return
			case es.outputCh <- struct{}{}: // Pass the event to the output
			}
		}
	}
}

// eventSupported checks if any notification event needs to be triggered
func (es *eventSubscription) eventSupported(
	messageType proto.MessageType,
	view *proto.View,
	totalMessages int,
) bool {
	// The heights must match
	if view.Height != es.details.View.Height {
		return false
	}

	// The rounds must match
	if view.Round != es.details.View.Round {
		return false
	}

	// The type of message must match
	if messageType != es.details.MessageType {
		return false
	}

	// The total number of messages must be
	// greater of equal to the subscription threshold
	return totalMessages >= es.details.NumMessages
}

// pushEvent sends the event off for processing by the subscription. [NON-BLOCKING]
func (es *eventSubscription) pushEvent(
	messageType proto.MessageType,
	view *proto.View,
	totalMessages int,
) {
	if es.eventSupported(messageType, view, totalMessages) {
		select {
		case es.notifyCh <- struct{}{}: // Notify the worker thread
		default:
		}
	}
}
