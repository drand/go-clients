package client

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/drand/drand/v2/common/log"
	clientMock "github.com/drand/go-clients/client/mock"
	"github.com/drand/go-clients/client/test/result/mock"
	"github.com/drand/go-clients/drand"
)

// waitForSpeedTest waits until all clients have had their initial speed test.
func waitForSpeedTest(t *testing.T, c drand.Client, timeout time.Duration) {
	t.Helper()
	oc, ok := c.(*optimizingClient)
	if !ok {
		t.Fatal("client is not an optimizing client")
	}

	timedOut := time.NewTimer(timeout)
	defer timedOut.Stop()
	for {
		oc.RLock()
		tested := true
		for _, s := range oc.stats {
			// all RTT's are zero until a speed test has been done
			if s.rtt == 0 {
				tested = false
				break
			}
		}
		oc.RUnlock()

		if tested {
			return
		}

		// try again in a bit...
		zzz := time.NewTimer(time.Millisecond * 100)
		select {
		case <-zzz.C:
		case <-timedOut.C:
			zzz.Stop()
			t.Fatal("timed out waiting for initial speed test to complete")
		}
	}
}

func expectRound(t *testing.T, res drand.Result, r uint64) {
	t.Helper()
	if res.GetRound() != r {
		t.Fatalf("expected round %v, got %v", r, res.GetRound())
	}
}

func closeClient(t *testing.T, c drand.Client) {
	t.Helper()
	err := c.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestOptimizingGet(t *testing.T) {
	c0 := clientMock.ClientWithResults(0, 5)
	c1 := clientMock.ClientWithResults(5, 8)

	c0.Delay = time.Millisecond * 100
	c1.Delay = time.Millisecond

	lg := log.New(nil, log.DebugLevel, true)
	oc, err := newOptimizingClient(lg, []drand.Client{c0, c1}, time.Second*5, 2, time.Minute*5, 0)
	if err != nil {
		t.Fatal(err)
	}
	oc.Start()
	defer closeClient(t, oc)

	waitForSpeedTest(t, oc, 10*time.Second)

	oc.Get(context.Background(), 0)
	// speed test will consume round 0 and 5 from c0 and c1
	// then c1 will be used because it's faster
	expectRound(t, latestResult(t, oc), 6) // round 6 from c1 and round 1 from c0 (discarded)
	expectRound(t, latestResult(t, oc), 7) // round 7 from c1 and round 2 from c0 (discarded)
	expectRound(t, latestResult(t, oc), 3) // c1 error (no results left), round 3 from c0
	expectRound(t, latestResult(t, oc), 4) // round 4 from c0
}

func TestOptimizingWatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c0 := clientMock.ClientWithResults(0, 5)
	c1 := clientMock.ClientWithResults(5, 8)
	c2 := clientMock.ClientWithInfo(fakeChainInfo(t))

	wc1 := make(chan drand.Result, 5)
	c1.WatchCh = wc1

	c0.Delay = time.Millisecond

	lg := log.New(nil, log.DebugLevel, true)
	oc, err := newOptimizingClient(lg, []drand.Client{c0, c1, c2}, time.Second*5, 2, time.Minute*5, 0)
	if err != nil {
		t.Fatal(err)
	}
	oc.Start()
	defer closeClient(t, oc)

	waitForSpeedTest(t, oc, time.Minute)

	ch := oc.Watch(ctx)

	expectRound(t, nextResult(t, ch), 1) // round 1 from c0 (after 100ms)
	wc1 <- &mock.Result{Rnd: 2}
	expectRound(t, nextResult(t, ch), 2) // round 2 from c1 and round 2 from c0 (discarded)
	select {
	case <-ch:
		t.Fatal("should not get another watched result at this point")
	case <-time.After(50 * time.Millisecond):
	}
	wc1 <- &mock.Result{Rnd: 6}
	expectRound(t, nextResult(t, ch), 6) // round 6 from c1
}

func TestOptimizingWatchRetryOnClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var rnd uint64
	c := &clientMock.Client{
		// a single result for the speed test
		Results: []mock.Result{mock.NewMockResult(0)},
		// return a watch channel that yields one result then closes
		WatchF: func(context.Context) <-chan drand.Result {
			ch := make(chan drand.Result, 1)
			r := mock.NewMockResult(rnd)
			rnd++
			ch <- &r
			close(ch)
			return ch
		},
	}

	lg := log.New(nil, log.DebugLevel, true)
	oc, err := newOptimizingClient(lg, []drand.Client{c}, 0, 0, 0, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	oc.Start()
	defer closeClient(t, oc)

	waitForSpeedTest(t, oc, time.Minute)

	ch := oc.Watch(ctx)

	var i uint64
	for r := range ch {
		if r.GetRound() != i {
			t.Fatal("unexpected round number")
		}
		i++
		if i > 2 {
			break
		}
	}
}

func TestOptimizingWatchFailover(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	chainInfo := fakeChainInfo(t)

	var rndlk sync.Mutex
	var rnd uint64 = 1
	wf := func(context.Context) <-chan drand.Result {
		rndlk.Lock()
		defer rndlk.Unlock()
		ch := make(chan drand.Result, 1)
		r := mock.NewMockResult(rnd)
		rnd++
		if rnd < 5 {
			ch <- &r
		}
		close(ch)
		return ch
	}
	c1 := &clientMock.Client{
		Results: []mock.Result{mock.NewMockResult(0)},
		WatchF:  wf,
	}
	c2 := &clientMock.Client{
		Results: []mock.Result{mock.NewMockResult(0)},
		WatchF:  wf,
	}

	lg := log.New(nil, log.DebugLevel, true)
	oc, err := newOptimizingClient(lg, []drand.Client{clientMock.ClientWithInfo(chainInfo), c1, c2}, 0, 0, 0, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	oc.Start()
	defer closeClient(t, oc)

	waitForSpeedTest(t, oc, time.Minute)

	ch := oc.Watch(ctx)

	var i uint64 = 1
	for r := range ch {
		if r.GetRound() != i {
			t.Fatalf("unexpected round number %d vs %d", r.GetRound(), i)
		}
		i++
		if i > 5 {
			t.Fatal("there are a total of 4 rounds possible")
		}
	}
	if i < 3 {
		t.Fatalf("watching didn't flip / yield expected rounds. %d", i)
	}
}

func TestOptimizingRequiresClients(t *testing.T) {
	lg := log.New(nil, log.DebugLevel, true)
	_, err := newOptimizingClient(lg, []drand.Client{}, 0, 0, 0, 0)
	if err == nil {
		t.Fatal("expected err is nil but it shouldn't be")
	}
	if err.Error() != "missing clients" {
		t.Fatal("unexpected error", err)
	}
}

func TestOptimizingIsLogging(t *testing.T) {
	lg := log.New(nil, log.DebugLevel, true)
	oc, err := newOptimizingClient(lg, []drand.Client{&clientMock.Client{}}, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	oc.SetLog(lg)
}

func TestOptimizingIsCloser(t *testing.T) {
	lg := log.New(nil, log.DebugLevel, true)
	oc, err := newOptimizingClient(lg, []drand.Client{&clientMock.Client{}}, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	oc.Start()
	err = oc.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestOptimizingInfo(t *testing.T) {
	lg := log.New(nil, log.DebugLevel, true)
	chainInfo := fakeChainInfo(t)
	oc, err := newOptimizingClient(lg, []drand.Client{clientMock.ClientWithInfo(chainInfo)}, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	oc.Start()
	i, err := oc.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if i != chainInfo {
		t.Fatal("wrong chain info", i)
	}
}

func TestOptimizingRoundAt(t *testing.T) {
	lg := log.New(nil, log.DebugLevel, true)
	oc, err := newOptimizingClient(lg, []drand.Client{&clientMock.Client{}}, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	oc.Start()
	r := oc.RoundAt(time.Now()) // mock client returns 0 always
	if r != 0 {
		t.Fatal("unexpected round", r)
	}
}

func TestOptimizingClose(t *testing.T) {
	wg := sync.WaitGroup{}

	closeF := func() error {
		wg.Done()
		return nil
	}

	clients := []drand.Client{
		&clientMock.Client{WatchCh: make(chan drand.Result), CloseF: closeF},
		&clientMock.Client{WatchCh: make(chan drand.Result), CloseF: closeF},
	}

	wg.Add(len(clients))

	lg := log.New(nil, log.DebugLevel, true)
	oc, err := newOptimizingClient(lg, clients, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	err = oc.Close() // should close the underlying clients
	if err != nil {
		t.Fatal(err)
	}

	wg.Wait() // wait for underlying clients to close
}

func TestOptimizingGetWithEmpty(t *testing.T) {
	lg := log.New(nil, log.DebugLevel, true)
	oc, err := newOptimizingClient(lg, []drand.Client{EmptyClientWithInfo(nil)}, time.Second*5, 2, time.Minute*5, 0)
	if err != nil {
		t.Fatal(err)
	}
	oc.Start()
	defer closeClient(t, oc)

	waitForSpeedTest(t, oc, 10*time.Second)

	res, err := oc.Get(context.Background(), 0)
	if !errors.Is(err, drand.ErrEmptyClientUnsupportedGet) {
		t.Fatal(err)
	}
	if res != nil {
		t.Fatal("expected nil result")
	}
}
