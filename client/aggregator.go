package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/drand/drand/v2/common/log"
	"github.com/drand/go-clients/drand"
)

const (
	aggregatorWatchBuffer = 5
	// defaultAutoWatchRetry is the time after which the watch channel
	// created by the autoWatch is re-opened when no context error occurred.
	defaultAutoWatchRetry = time.Second * 30
)

// newWatchAggregator maintains state of consumers calling `Watch` so that a
// single `watch` request is made to the underlying client.
// There are 3 modes taken by this aggregator. If autowatch is set, a single `watch`
// will always be invoked on the provided client. If it is not set, but a `watch client`(wc)
// is passed, a `watch` will be run on the watch client in the absence of external watchers,
// which will swap watching over to the main client. If no watch client is set and autowatch is off
// then a single watch will only run when an external watch is requested.
func newWatchAggregator(l log.Logger, c, wc drand.Client, autoWatch bool, autoWatchRetry time.Duration) *watchAggregator {
	if autoWatchRetry == 0 {
		autoWatchRetry = defaultAutoWatchRetry
	}
	aggregator := &watchAggregator{
		Client:         c,
		passiveClient:  wc,
		autoWatch:      autoWatch,
		autoWatchRetry: autoWatchRetry,
		log:            l,
		subscribers:    make([]*subscriber, 0),
	}
	return aggregator
}

type subscriber struct {
	ctx  context.Context
	c    chan drand.Result
	once sync.Once
}

// close closes the subscriber channel exactly once, since both the distribute
// loop and the per-subscriber watchdog can retire a subscriber.
func (s *subscriber) close() {
	s.once.Do(func() { close(s.c) })
}

type watchAggregator struct {
	drand.Client
	passiveClient   drand.Client
	autoWatch       bool
	autoWatchRetry  time.Duration
	log             log.Logger
	cancelAutoWatch context.CancelFunc

	subscriberLock   sync.Mutex
	subscribers      []*subscriber
	cancelDistribute context.CancelFunc
	cancelPassive    context.CancelFunc
	// passiveToken identifies the currently running passive sink, so a sink
	// that ends can only clear its own cancel function and not a newer one's.
	passiveToken *int
}

// Start initiates auto watching if configured to do so.
// SetLog should not be called after Start.
func (c *watchAggregator) Start() {
	if c.autoWatch {
		c.startAutoWatch(true)
	} else if c.passiveClient != nil {
		c.startAutoWatch(false)
	}
}

// SetLog configures the client log output
func (c *watchAggregator) SetLog(l log.Logger) {
	c.log = l
}

// String returns the name of this client.
func (c *watchAggregator) String() string {
	return fmt.Sprintf("%s.(+aggregator)", c.Client)
}

func (c *watchAggregator) startAutoWatch(full bool) {
	ctx, cancel := context.WithCancel(context.Background())
	c.cancelAutoWatch = cancel
	go func() {
		for {
			var results <-chan drand.Result
			if full {
				results = c.Watch(ctx)
			} else if c.passiveClient != nil {
				results = c.passiveWatch(ctx)
			}
		LOOP:
			for {
				select {
				case _, ok := <-results:
					if !ok {
						c.log.Infow("", "watch_aggregator", "auto watch ended")
						break LOOP
					}
				case <-ctx.Done():
					return
				}
			}
			if c.autoWatchRetry < 0 {
				return
			}
			t := time.NewTimer(c.autoWatchRetry)
			select {
			case <-t.C:
			case <-ctx.Done():
				if !t.Stop() {
					<-t.C
				}
			}
			c.log.Infow("", "watch_aggregator", "retrying auto watch")
		}
	}()
}

// passiveWatch is a degraded form of watch, where watch only hits the 'passive client'
// unless distribution is actually needed.
func (c *watchAggregator) passiveWatch(ctx context.Context) <-chan drand.Result {
	c.subscriberLock.Lock()
	defer c.subscriberLock.Unlock()

	if c.cancelPassive != nil {
		c.log.Warnw("", "watch_aggregator", "only support one passive watch")
		return nil
	}

	wc := make(chan drand.Result)
	if len(c.subscribers) == 0 {
		ctx, cancel := context.WithCancel(ctx)
		token := new(int)
		c.cancelPassive = cancel
		c.passiveToken = token
		go c.sink(c.passiveClient.Watch(ctx), wc, token)
	} else {
		// trigger the startAutowatch to retry on backoff
		close(wc)
	}
	return wc
}

func (c *watchAggregator) Watch(ctx context.Context) <-chan drand.Result {
	c.subscriberLock.Lock()
	defer c.subscriberLock.Unlock()

	sub := &subscriber{ctx: ctx, c: make(chan drand.Result, aggregatorWatchBuffer)}
	c.subscribers = append(c.subscribers, sub)

	if len(c.subscribers) == 1 {
		if c.cancelPassive != nil {
			c.cancelPassive()
			c.cancelPassive = nil
			c.passiveToken = nil
		}
		// The upstream watch is deliberately not derived from this subscriber's
		// context: it is shared by every subscriber, so tying its lifetime to
		// whoever happened to subscribe first would tear down the others when
		// that one goes away. It is cancelled once the last subscriber leaves,
		// and by Close.
		wctx, cancel := context.WithCancel(context.Background())
		c.cancelDistribute = cancel
		go c.distribute(c.Client.Watch(wctx), cancel)
	}

	// Each subscriber is retired on its own context, independently of the others.
	go func() {
		<-ctx.Done()
		c.removeSubscriber(sub)
	}()

	return sub.c
}

// removeSubscriber retires a single subscriber, closing its channel. When the
// last subscriber leaves, the shared upstream watch is cancelled.
func (c *watchAggregator) removeSubscriber(sub *subscriber) {
	c.subscriberLock.Lock()
	defer c.subscriberLock.Unlock()

	for i, s := range c.subscribers {
		if s == sub {
			c.subscribers = append(c.subscribers[:i], c.subscribers[i+1:]...)
			break
		}
	}
	sub.close()

	if len(c.subscribers) == 0 && c.cancelDistribute != nil {
		c.cancelDistribute()
		c.cancelDistribute = nil
	}
}

func (c *watchAggregator) sink(in <-chan drand.Result, out chan drand.Result, token *int) {
	defer close(out)
	for range in {
		continue
	}

	// Clear the passive watch state now that this stream has ended, otherwise
	// passiveWatch keeps seeing a non-nil cancelPassive, refuses to start a new
	// passive watch and returns nil, which leaves startAutoWatch selecting on a
	// nil channel forever.
	c.subscriberLock.Lock()
	defer c.subscriberLock.Unlock()
	if c.passiveToken == token {
		if c.cancelPassive != nil {
			c.cancelPassive()
		}
		c.cancelPassive = nil
		c.passiveToken = nil
	}
}

func (c *watchAggregator) distribute(in <-chan drand.Result, cancel context.CancelFunc) {
	defer cancel()
	for {
		m, ok := <-in

		c.subscriberLock.Lock()

		if !ok {
			// the upstream watch ended, so no further results are coming
			for _, s := range c.subscribers {
				s.close()
			}
			c.subscribers = nil
			c.cancelDistribute = nil
			c.subscriberLock.Unlock()
			return
		}

		if len(c.subscribers) == 0 {
			c.subscriberLock.Unlock()
			c.log.Warnw("", "watch_aggregator", "no subscribers to distribute results to")
			return
		}

		for _, s := range c.subscribers {
			if m != nil {
				select {
				case s.c <- m:
				default:
					c.log.Warnw("", "watch_aggregator", "dropped watch message to subscriber. full channel")
				}
			}
		}
		c.subscriberLock.Unlock()
	}
}

func (c *watchAggregator) Close() error {
	err := c.Client.Close()
	if c.cancelAutoWatch != nil {
		c.cancelAutoWatch()
	}

	c.subscriberLock.Lock()
	if c.cancelDistribute != nil {
		c.cancelDistribute()
		c.cancelDistribute = nil
	}
	if c.cancelPassive != nil {
		c.cancelPassive()
		c.cancelPassive = nil
		c.passiveToken = nil
	}
	c.subscriberLock.Unlock()

	return err
}
