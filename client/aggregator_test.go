package client

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/drand/drand/v2/common/log"
	clientMock "github.com/drand/go-clients/client/mock"
	"github.com/drand/go-clients/client/test/result/mock"
	"github.com/drand/go-clients/drand"
)

func TestAggregatorClose(t *testing.T) {
	wg := sync.WaitGroup{}
	wg.Add(1)

	c := &clientMock.Client{
		WatchCh: make(chan drand.Result),
		CloseF: func() error {
			wg.Done()
			return nil
		},
	}

	ac := newWatchAggregator(log.New(nil, log.DebugLevel, true), c, nil, true, 0)

	err := ac.Close() // should cancel the autoWatch and close the underlying client
	if err != nil {
		t.Fatal(err)
	}

	wg.Wait() // wait for underlying client to close
}

func TestAggregatorPassive(t *testing.T) {
	wg := sync.WaitGroup{}
	wg.Add(1)

	c := &clientMock.Client{
		WatchCh: make(chan drand.Result, 1),
		CloseF: func() error {
			wg.Done()
			return nil
		},
	}

	wc := &clientMock.Client{
		WatchCh: make(chan drand.Result, 1),
		CloseF: func() error {
			return nil
		},
	}

	ac := newWatchAggregator(log.New(nil, log.DebugLevel, true), c, wc, false, 0)

	wc.WatchCh <- &mock.Result{Rnd: 1234}
	c.WatchCh <- &mock.Result{Rnd: 5678}

	ac.Start()

	time.Sleep(50 * time.Millisecond)

	zzz := time.NewTimer(time.Millisecond * 50)
	select {
	case w := <-wc.WatchCh:
		t.Fatalf("passive watch should be drained, but got %v", w)
	case <-zzz.C:
	}

	zzz = time.NewTimer(time.Millisecond * 50)
	select {
	case <-c.WatchCh:
	case <-zzz.C:
		t.Fatalf("active watch should not have been called but was")
	}

	err := ac.Close()
	if err != nil {
		t.Fatal(err)
	}

	wg.Wait()
}

// TestAggregatorSubscribersAreIndependent ensures cancelling one subscriber's
// context does not tear down the others. The distribute loop used to select on
// the first subscriber's context only, so when that one went away every other
// subscriber's channel was closed alongside it.
func TestAggregatorSubscribersAreIndependent(t *testing.T) {
	watchCh := make(chan drand.Result, 1)
	c := &clientMock.Client{WatchCh: watchCh}

	ac := newWatchAggregator(log.New(nil, log.DebugLevel, true), c, nil, false, 0)
	defer ac.Close()

	ctxFirst, cancelFirst := context.WithCancel(context.Background())
	first := ac.Watch(ctxFirst)

	ctxSecond, cancelSecond := context.WithCancel(context.Background())
	defer cancelSecond()
	second := ac.Watch(ctxSecond)

	// retire the first subscriber and wait for it to be reaped
	cancelFirst()
	for range first { //nolint:revive // draining until closed is the point
	}

	// the second subscriber must still be served
	res := &mock.Result{Rnd: 42, Rand: []byte{0x42}}
	select {
	case watchCh <- res:
	case <-time.After(time.Second):
		t.Fatal("timed out publishing a result")
	}

	select {
	case got, ok := <-second:
		if !ok {
			t.Fatal("second subscriber was closed when the first one was cancelled")
		}
		if got.GetRound() != 42 {
			t.Fatalf("expected round 42, got %d", got.GetRound())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("second subscriber did not receive the result")
	}
}
