package messages

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/0xPolygon/go-ibft/messages/proto"
)

func TestEventManager_signalEvent(t *testing.T) {
	t.Parallel()

	var (
		baseDetails = SubscriptionDetails{
			MessageType: proto.MessageType_PREPARE,
			View: &proto.View{
				Height: 0,
				Round:  0,
			},
			MinNumMessages: 1,
		}

		baseEventType = baseDetails.MessageType
		baseEventView = &proto.View{
			Height: baseDetails.View.Height,
			Round:  baseDetails.View.Round,
		}
	)

	// setupEventManagerAndSubscription creates new eventManager and a subscription
	setupEventManagerAndSubscription := func(t *testing.T) (*eventManager, *Subscription) {
		t.Helper()

		em := newEventManager()

		t.Cleanup(func() {
			em.close()
		})

		subscription := em.subscribe(baseDetails)

		t.Cleanup(func() {
			em.cancelSubscription(subscription.ID)
		})

		return em, subscription
	}

	// emitEvent sends a event to eventManager and close doneCh after signalEvent completes
	emitEvent := func(
		t *testing.T,
		em *eventManager,
		eventType proto.MessageType,
		eventView *proto.View,
	) <-chan struct{} {
		t.Helper()

		doneCh := make(chan struct{})

		go func() {
			t.Helper()

			defer close(doneCh)

			em.signalEvent(
				eventType,
				eventView,
			)
		}()

		return doneCh
	}

	// testSubscriptionData checks the data sent to subscription
	testSubscriptionData := func(
		t *testing.T,
		sub *Subscription,
		expectedSignals []uint64,
	) <-chan struct{} {
		t.Helper()

		doneCh := make(chan struct{})

		go func() {
			t.Helper()

			defer close(doneCh)

			actualSignals := make([]uint64, 0)
			for sig := range sub.SubCh {
				actualSignals = append(actualSignals, sig)
			}

			assert.Equal(t, expectedSignals, actualSignals)
		}()

		return doneCh
	}

	// closeSubscription closes subscription manually
	// because cancelSubscription might be unable
	// due to mutex locking during tests
	closeSubscription := func(
		t *testing.T,
		em *eventManager,
		sub *Subscription,
	) {
		t.Helper()

		close(em.subscriptions[sub.ID].doneCh)
		delete(em.subscriptions, sub.ID)
	}

	t.Run("should exit before locking subscriptionsLock if numSubscriptions is zero", func(t *testing.T) {
		t.Parallel()

		em, sub := setupEventManagerAndSubscription(t)

		// overwrite numSubscription
		atomic.StoreInt64(&em.numSubscriptions, 0)
		// shouldn't be locked by mutex thanks to early return
		em.subscriptionsLock.Lock()
		t.Cleanup(func() {
			em.subscriptionsLock.Unlock()
		})

		doneEmitCh := emitEvent(t, em, baseEventType, baseEventView)
		doneTestSubCh := testSubscriptionData(t, sub, []uint64{})

		// should exit by early return
		select {
		case <-doneEmitCh:
		case <-time.After(5 * time.Second):
			t.Errorf("signalEvent shouldn't be lock, but it was locked")

			return
		}

		closeSubscription(t, em, sub)
		<-doneTestSubCh
	})

	t.Run("should be locked by other write lock", func(t *testing.T) {
		t.Parallel()

		em, sub := setupEventManagerAndSubscription(t)

		// should be locked by other write lock
		em.subscriptionsLock.Lock()
		t.Cleanup(func() {
			em.subscriptionsLock.Unlock()
		})

		doneCh := emitEvent(t, em, baseEventType, baseEventView)
		doneTestSubCh := testSubscriptionData(t, sub, []uint64{})

		select {
		case <-doneCh:
			t.Errorf("signalEvent is not locked")

			return
		case <-time.After(5 * time.Second):
		}

		closeSubscription(t, em, sub)
		<-doneTestSubCh
	})

	t.Run("should not be locked by other read lock", func(t *testing.T) {
		t.Parallel()

		em, sub := setupEventManagerAndSubscription(t)

		// shouldn't be locked by mutex of read-lock
		em.subscriptionsLock.RLock()
		t.Cleanup(func() {
			em.subscriptionsLock.RUnlock()
		})

		doneCh := emitEvent(t, em, baseEventType, baseEventView)
		doneTestSubCh := testSubscriptionData(t, sub, []uint64{0})

		select {
		case <-doneCh:
		case <-time.After(5 * time.Second):
			t.Errorf("signalEvent is locked")

			return
		}

		closeSubscription(t, em, sub)
		<-doneTestSubCh
	})

	t.Run("should not notify if the event is different the one expected by subscription", func(t *testing.T) {
		t.Parallel()

		em, sub := setupEventManagerAndSubscription(t)

		doneCh := emitEvent(t, em, proto.MessageType_COMMIT, baseEventView)
		doneTestSubCh := testSubscriptionData(t, sub, []uint64{})

		select {
		case <-doneCh:
		case <-time.After(5 * time.Second):
			t.Errorf("signalEvent is locked")

			return
		}

		closeSubscription(t, em, sub)
		<-doneTestSubCh
	})
}

func TestEventManager_SubscribeCancel(t *testing.T) {
	t.Parallel()

	baseDetails := SubscriptionDetails{
		MessageType: proto.MessageType_PREPARE,
		View: &proto.View{
			Height: 0,
			Round:  0,
		},
		MinNumMessages: 1,
	}

	numSubscriptions := 10
	subscriptions := make([]*Subscription, numSubscriptions)

	idMap := make(map[SubscriptionID]bool)

	em := newEventManager()
	defer em.close()

	// Create the subscriptions
	for i := 0; i < numSubscriptions; i++ {
		subscriptions[i] = em.subscribe(baseDetails)

		// Check that the number is up-to-date
		assert.Equal(t, int64(i+1), em.numSubscriptions)

		// Check if a duplicate ID has been issued
		if _, ok := idMap[subscriptions[i].ID]; ok {
			t.Fatalf("Duplicate ID entry")
		} else {
			idMap[subscriptions[i].ID] = true
		}
	}

	quitCh := make(chan struct{}, 1)
	defer func() {
		quitCh <- struct{}{}
	}()

	go func() {
		for {
			em.signalEvent(baseDetails.MessageType, baseDetails.View)

			select {
			case <-quitCh:
				return
			default:
			}
		}
	}()

	// Cancel them concurrently
	for _, subscription := range subscriptions {
		em.cancelSubscription(subscription.ID)
	}

	// Check that the number is up-to-date
	assert.Equal(t, int64(0), em.numSubscriptions)
}

func TestEventManager_SubscribeClose(t *testing.T) {
	t.Parallel()

	baseDetails := SubscriptionDetails{
		MessageType: proto.MessageType_PREPARE,
		View: &proto.View{
			Height: 0,
			Round:  0,
		},
		MinNumMessages: 1,
	}

	numSubscriptions := 10
	subscriptions := make([]*Subscription, numSubscriptions)

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
		if _, more := <-subscription.SubCh; more {
			t.Fatalf("SubscriptionDetails channel not closed for index %d", indx)
		}
	}
}
