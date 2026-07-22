package lp2p

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/v2/common/log"
	pbdrand "github.com/drand/drand/v2/protobuf/drand"
)

// newBareClient returns a Client with only the state Sub/unsubscribe touch, so
// the subscription bookkeeping can be tested without standing up a libp2p host
// and a relay node.
func newBareClient() *Client {
	c := &Client{log: log.New(nil, log.DebugLevel, true)}
	c.subs.M = make(map[*int]*subscription)
	return c
}

// closeAllSubs mirrors what the background goroutine in NewWithPubsub does when
// the client is closed: it closes every registered subscription and drops them.
func closeAllSubs(c *Client) {
	c.subs.Lock()
	defer c.subs.Unlock()
	for _, sub := range c.subs.M {
		sub.close()
	}
	c.subs.M = make(map[*int]*subscription)
}

// TestUnsubAfterCloseDoesNotPanic covers the ordering produced by the idiomatic
// `defer c.Close()` + `defer cancel()` pair: Close() runs first and closes every
// subscription, then the cancelled Watch context invokes the unsubscribe
// function. Both used to close the same channel, panicking the process.
func TestUnsubAfterCloseDoesNotPanic(t *testing.T) {
	c := newBareClient()
	ch := make(chan pbdrand.PublicRandResponse, 1)
	end := c.Sub(ch)

	closeAllSubs(c)

	require.NotPanics(t, func() { end() }, "unsubscribing after the client closed the channel must not panic")

	_, ok := <-ch
	require.False(t, ok, "channel should be closed")
}

// TestUnsubIsIdempotent ensures calling the unsubscribe function more than once
// is safe, since callers may both defer it and call it explicitly.
func TestUnsubIsIdempotent(t *testing.T) {
	c := newBareClient()
	ch := make(chan pbdrand.PublicRandResponse, 1)
	end := c.Sub(ch)

	end()
	require.NotPanics(t, func() { end() }, "second unsubscribe must not panic")
}

// TestCloseAfterUnsubDoesNotPanic covers the reverse ordering: the subscriber
// unsubscribes first and the client is closed afterwards.
func TestCloseAfterUnsubDoesNotPanic(t *testing.T) {
	c := newBareClient()
	ch := make(chan pbdrand.PublicRandResponse, 1)
	end := c.Sub(ch)

	end()
	require.NotPanics(t, func() { closeAllSubs(c) }, "closing after unsubscribe must not panic")
}
