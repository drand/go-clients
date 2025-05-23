package lp2p

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	dnsaddr "github.com/multiformats/go-multiaddr-dns"
	"google.golang.org/protobuf/proto"

	"github.com/drand/drand/v2/common/chain"
	"github.com/drand/drand/v2/common/log"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/drand/v2/protobuf/drand"
	"github.com/drand/go-clients/client"
	drandi "github.com/drand/go-clients/drand"
)

var _ drandi.LoggingClient = &Client{}

// WatchBufferSize controls how many incoming messages can be in-flight until they start
// to be dropped by the library when using Client.Watch
var WatchBufferSize = 100

// Client is a concrete pubsub client implementation
type Client struct {
	cancel func()
	latest uint64
	cache  client.Cache
	log    log.Logger

	subs struct {
		sync.Mutex
		M map[*int]chan drand.PublicRandResponse
	}
}

// SetLog configures the client log output
func (c *Client) SetLog(l log.Logger) {
	c.log = l
}

// WithPubsub provides an option for integrating pubsub notification
// into a drand client.
func WithPubsub(ps *pubsub.PubSub) client.Option {
	return client.WithWatcher(func(l log.Logger, info *chain.Info, cache client.Cache) (client.Watcher, error) {
		c, err := NewWithPubsub(l, ps, info, cache)
		if err != nil {
			return nil, err
		}
		return c, nil
	})
}

// PubSubTopic generates a drand pubsub topic from a chain hash.
func PubSubTopic(h string) string {
	return fmt.Sprintf("/drand/pubsub/v0.0.0/%s", h)
}

// NewWithPubsub creates a gossip randomness client. If the logger l is nil, it will default to
// a default Logger,
//
//nolint:funlen,gocyclo // This is a long line
func NewWithPubsub(l log.Logger, ps *pubsub.PubSub, info *chain.Info, cache client.Cache) (*Client, error) {
	if info == nil {
		return nil, fmt.Errorf("no chain supplied for joining")
	}

	if l == nil {
		l = log.DefaultLogger()
	}

	scheme, err := crypto.SchemeFromName(info.Scheme)
	if err != nil {
		l.Errorw("invalid scheme in info", "info", info, "scheme", info.Scheme, "err", err)

		return nil, fmt.Errorf("invalid scheme in info: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{
		cancel: cancel,
		cache:  cache,
		log:    l,
	}

	chainHash := hex.EncodeToString(info.Hash())
	topic := PubSubTopic(chainHash)
	if err := ps.RegisterTopicValidator(topic, randomnessValidator(info, cache, c)); err != nil {
		cancel()
		return nil, fmt.Errorf("creating topic: %w", err)
	}
	t, err := ps.Join(topic)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("joining pubsub: %w", err)
	}
	s, err := t.Subscribe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("subscribe: %w", err)
	}

	c.subs.M = make(map[*int]chan drand.PublicRandResponse)

	go func() {
		for {
			msg, err := s.Next(ctx)
			if ctx.Err() != nil {
				c.log.Debugw("NewPubSub closing because context was canceled", "msg", msg, "err", ctx.Err())

				s.Cancel()
				err := t.Close()
				if err != nil {
					c.log.Errorw("NewPubSub closing goroutine for topic", "err", err)
				}

				c.subs.Lock()
				for _, ch := range c.subs.M {
					close(ch)
				}
				c.subs.M = make(map[*int]chan drand.PublicRandResponse)
				c.subs.Unlock()
				return
			}
			if err != nil {
				c.log.Warnw("", "gossip client", "topic.Next error", "err", err)
				continue
			}

			var rand drand.PublicRandResponse
			err = proto.Unmarshal(msg.Data, &rand)
			if err != nil {
				c.log.Warnw("", "gossip client", "unmarshal random error", "err", err)
				continue
			}

			err = scheme.VerifyBeacon(&rand, info.PublicKey)
			if err != nil {
				c.log.Errorw("invalid signature for beacon", "round", rand.GetRound(), "err", err)
				continue
			}

			if c.latest >= rand.Round {
				c.log.Debugw("received round older than the latest previously received one", "latest", c.latest, "round", rand.Round)
				continue
			}
			c.latest = rand.Round

			c.log.Debugw("newPubSub broadcasting round to listeners", "round", rand.Round)
			c.subs.Lock()
			for _, ch := range c.subs.M {
				select {
				case ch <- rand:
				default:
					c.log.Warnw("", "gossip client", "randomness notification dropped due to a full channel")
				}
			}
			c.subs.Unlock()
			c.log.Debugw("newPubSub finished broadcasting round to listeners", "round", rand.Round)
		}
	}()

	return c, nil
}

// UnsubFunc is a cancel function for pubsub subscription
type UnsubFunc func()

// Sub subscribes to notifications about new randomness.
// Client instance owns the channel after it is passed to Sub function,
// thus the channel should not be closed by library user
//
// It is recommended to use a buffered channel. If the channel is full,
// notification about randomness will be dropped.
//
// Notification channels will be closed when the client is Closed
func (c *Client) Sub(ch chan drand.PublicRandResponse) UnsubFunc {
	id := new(int)
	c.subs.Lock()
	c.subs.M[id] = ch
	c.subs.Unlock()
	return func() {
		c.log.Debugw("closing sub")
		c.subs.Lock()
		delete(c.subs.M, id)
		close(ch)
		c.subs.Unlock()
	}
}

// Watch implements the client.Watcher interface
func (c *Client) Watch(ctx context.Context) <-chan drandi.Result {
	innerCh := make(chan drand.PublicRandResponse, WatchBufferSize)
	outerCh := make(chan drandi.Result, WatchBufferSize)
	end := c.Sub(innerCh)

	w := sync.WaitGroup{}
	w.Add(1)

	go func() {
		defer close(outerCh)

		w.Done()

		for {
			select {
			case resp, ok := <-innerCh: //nolint:govet
				if !ok {
					c.log.Debugw("innerCh closed")
					return
				}
				dat := &client.RandomData{
					Rnd:               resp.GetRound(),
					Random:            crypto.RandomnessFromSignature(resp.GetSignature()),
					Sig:               resp.GetSignature(),
					PreviousSignature: resp.GetPreviousSignature(),
				}
				if c.cache != nil {
					c.cache.Add(resp.GetRound(), dat)
				}
				select {
				case outerCh <- dat:
					c.log.Debugw("processed random beacon", "round", dat.GetRound())
				default:
					c.log.Warnw("", "gossip client", "randomness notification dropped due to a full channel", "round", dat.GetRound())
				}
			case <-ctx.Done():
				c.log.Debugw("client.Watch done")
				end()
				c.log.Debugw("client.Watch finished draining the innerCh")
				return
			}
		}
	}()

	w.Wait()

	return outerCh
}

// Close stops Client, cancels PubSub subscription and closes the topic.
func (c *Client) Close() error {
	c.cancel()
	return nil
}

// NewPubsub constructs a basic libp2p pubsub module for use with the drand client.
// The local libp2p host is returned as well to allow to properly close it once done.
func NewPubsub(ctx context.Context, listenAddr string, relayAddrs []string) (*pubsub.PubSub, host.Host, error) {
	h, err := libp2p.New(libp2p.ListenAddrStrings(listenAddr))
	if err != nil {
		return nil, nil, err
	}

	peers := make([]peer.AddrInfo, 0, len(relayAddrs))
	for _, relayAddr := range relayAddrs {
		// resolve the relay multiaddr to peers' AddrInfo
		mas, err := dnsaddr.Resolve(ctx, multiaddr.StringCast(relayAddr))
		if err != nil {
			return nil, nil, fmt.Errorf("dnsaddr.Resolve error: %w", err)
		}
		for _, ma := range mas {
			relayAi, err := peer.AddrInfoFromP2pAddr(ma)
			if err != nil {
				h.Close()
				return nil, nil, fmt.Errorf("peer.AddrInfoFromP2pAddr error: %w", err)
			}
			peers = append(peers, *relayAi)
		}
	}

	ps, err := pubsub.NewGossipSub(ctx, h, pubsub.WithDirectPeers(peers))
	return ps, h, err
}
