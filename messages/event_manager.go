package messages

import (
	"github.com/0xPolygon/go-ibft/messages/proto"
	"github.com/google/uuid"
	"sync"
	"sync/atomic"
)

type eventManager struct {
	subscriptions     map[SubscriptionID]*eventSubscription
	subscriptionsLock sync.RWMutex
	numSubscriptions  int64
}

func newEventManager() *eventManager {
	return &eventManager{
		subscriptions:    make(map[SubscriptionID]*eventSubscription),
		numSubscriptions: 0,
	}
}

type SubscriptionID int32

type Subscription struct {
	id                  SubscriptionID
	subscriptionChannel chan uint64
}

func NewSubscription(id SubscriptionID, ch chan uint64) *Subscription {
	return &Subscription{
		id:                  id,
		subscriptionChannel: ch,
	}
}

func (sr *Subscription) GetCh() chan uint64 {
	return sr.subscriptionChannel
}

func (sr *Subscription) GetID() SubscriptionID {
	return sr.id
}

type SubscriptionDetails struct {
	MessageType proto.MessageType
	View        *proto.View
	NumMessages int
	HasMinRound bool
}

// subscribe registers a new listener for message events
func (em *eventManager) subscribe(details SubscriptionDetails) *Subscription {
	em.subscriptionsLock.Lock()
	defer em.subscriptionsLock.Unlock()

	id := uuid.New().ID()
	subscription := &eventSubscription{
		details:  details,
		outputCh: make(chan uint64, 1),
		doneCh:   make(chan struct{}),
		notifyCh: make(chan uint64, 1),
	}

	em.subscriptions[SubscriptionID(id)] = subscription

	go subscription.runLoop()

	atomic.AddInt64(&em.numSubscriptions, 1)

	return &Subscription{
		id:                  SubscriptionID(id),
		subscriptionChannel: subscription.outputCh,
	}
}

// cancelSubscription stops a subscription for message events
func (em *eventManager) cancelSubscription(id SubscriptionID) {
	em.subscriptionsLock.Lock()
	defer em.subscriptionsLock.Unlock()

	if subscription, ok := em.subscriptions[id]; ok {
		subscription.close()
		delete(em.subscriptions, id)
		atomic.AddInt64(&em.numSubscriptions, -1)
	}
}

// close stops the event manager, effectively cancelling all subscriptions
func (em *eventManager) close() {
	em.subscriptionsLock.Lock()
	defer em.subscriptionsLock.Unlock()

	for _, subscription := range em.subscriptions {
		subscription.close()
	}

	atomic.StoreInt64(&em.numSubscriptions, 0)
}

// signalEvent is a helper method for alerting listeners of a new message event
func (em *eventManager) signalEvent(
	messageType proto.MessageType,
	view *proto.View,
	totalMessages int,
) {
	if atomic.LoadInt64(&em.numSubscriptions) < 1 {
		// No reason to lock the subscriptions map
		// if no subscriptions exist
		return
	}

	em.subscriptionsLock.RLock()
	defer em.subscriptionsLock.RUnlock()

	for _, subscription := range em.subscriptions {
		subscription.pushEvent(
			messageType,
			view,
			totalMessages,
		)
	}
}
