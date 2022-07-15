package messages

import (
	"github.com/Trapesys/go-ibft/messages/proto"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestEventManager_SubscribeCancel(t *testing.T) {
	t.Parallel()

	numSubscriptions := 10
	subscriptions := make([]*Subscription, numSubscriptions)
	baseDetails := SubscriptionDetails{
		MessageType: proto.MessageType_PREPARE,
		View: &proto.View{
			Height: 0,
			Round:  0,
		},
		NumMessages: 1,
	}

	IDMap := make(map[SubscriptionID]bool)

	em := newEventManager()
	defer em.close()

	// Create the subscriptions
	for i := 0; i < numSubscriptions; i++ {
		subscriptions[i] = em.subscribe(baseDetails)

		// Check that the number is up-to-date
		assert.Equal(t, int64(i+1), em.numSubscriptions)

		// Check if a duplicate ID has been issued
		if _, ok := IDMap[subscriptions[i].GetID()]; ok {
			t.Fatalf("Duplicate ID entry")
		} else {
			IDMap[subscriptions[i].GetID()] = true
		}
	}

	quitCh := make(chan struct{}, 1)
	defer func() {
		quitCh <- struct{}{}
	}()

	go func() {
		for {
			em.signalEvent(baseDetails.MessageType, baseDetails.View, baseDetails.NumMessages)

			select {
			case <-quitCh:
				return
			default:
			}
		}
	}()

	// Cancel them concurrently
	for _, subscription := range subscriptions {
		em.cancelSubscription(subscription.GetID())
	}

	// Check that the number is up-to-date
	assert.Equal(t, int64(0), em.numSubscriptions)
}

func TestEventManager_SubscribeClose(t *testing.T) {
	t.Parallel()

	numSubscriptions := 10
	subscriptions := make([]*Subscription, numSubscriptions)
	baseDetails := SubscriptionDetails{
		MessageType: proto.MessageType_PREPARE,
		View: &proto.View{
			Height: 0,
			Round:  0,
		},
		NumMessages: 1,
	}

	em := newEventManager()

	// Create the subscriptions
	for i := 0; i < numSubscriptions; i++ {
		subscriptions[i] = em.subscribe(baseDetails)

		// Check that the number is up-to-date
		assert.Equal(t, int64(i+1), em.numSubscriptions)
	}

	// Close off the event manager
	em.close()
	assert.Equal(t, int64(0), em.numSubscriptions)

	// Check if the subscription channels are closed
	for indx, subscription := range subscriptions {
		if _, more := <-subscription.GetCh(); more {
			t.Fatalf("SubscriptionDetails channel not closed for index %d", indx)
		}
	}
}
