package client

import (
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
