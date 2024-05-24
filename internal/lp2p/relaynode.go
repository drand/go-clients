package lp2p

import (
	"context"
	"fmt"
	"time"

	bds "github.com/ipfs/go-ds-badger2"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	ma "github.com/multiformats/go-multiaddr"
	"google.golang.org/protobuf/proto"

	client2 "github.com/drand/drand-cli/client"
	"github.com/drand/drand/v2/common/client"
	"github.com/drand/drand/v2/common/log"
	"github.com/drand/drand/v2/protobuf/drand"
)

// GossipRelayConfig configures a gossip-relay relay node.
type GossipRelayConfig struct {
	// ChainHash is a hash that uniquely identifies the drand chain.
	ChainHash    string
	PeerWith     []string
	Addr         string
	DataDir      string
	IdentityPath string
	CertPath     string
	Insecure     bool
	Client       client.Client
}

// GossipRelayNode is a gossip-relay relay runtime.
type GossipRelayNode struct {
	l         log.Logger
	bootstrap []ma.Multiaddr
	ds        *bds.Datastore
	priv      crypto.PrivKey
	h         host.Host
	ps        *pubsub.PubSub
	t         *pubsub.Topic
	addrs     []ma.Multiaddr
	done      chan struct{}
}

// NewGossipRelayNode starts a new gossip-relay relay node.
func NewGossipRelayNode(l log.Logger, cfg *GossipRelayConfig) (*GossipRelayNode, error) {
	if cfg.Client == nil {
		return nil, fmt.Errorf("no client supplying randomness supplied")
	}

	bootstrap, err := ParseMultiaddrSlice(cfg.PeerWith)
	if err != nil {
		return nil, fmt.Errorf("parsing peer-with: %w", err)
	}

	ds, err := bds.NewDatastore(cfg.DataDir, nil)
	if err != nil {
		return nil, fmt.Errorf("opening datastore: %w", err)
	}

	priv, err := LoadOrCreatePrivKey(cfg.IdentityPath, l)
	if err != nil {
		return nil, fmt.Errorf("loading p2p key: %w", err)
	}

	h, ps, err := ConstructHost(ds, priv, cfg.Addr, bootstrap, l)
	if err != nil {
		return nil, fmt.Errorf("constructing host: %w", err)
	}

	addrs, err := h.Network().InterfaceListenAddresses()
	if err != nil {
		return nil, fmt.Errorf("getting InterfaceListenAddresses: %w", err)
	}

	for _, a := range addrs {
		l.Infow("", "relay_node", "has addr", "addr", fmt.Sprintf("%s/p2p/%s", a, h.ID()))
	}
	l.Infow("Joining PubSubTopic", "chainhash", cfg.ChainHash)
	t, err := ps.Join(PubSubTopic(cfg.ChainHash))
	if err != nil {
		return nil, fmt.Errorf("joining topic: %w", err)
	}

	g := &GossipRelayNode{
		l:         l,
		bootstrap: bootstrap,
		ds:        ds,
		priv:      priv,
		h:         h,
		ps:        ps,
		t:         t,
		addrs:     addrs,
		done:      make(chan struct{}),
	}

	go g.background(cfg.Client)

	return g, nil
}

// Multiaddrs returns the gossipsub multiaddresses of this relay node.
func (g *GossipRelayNode) Multiaddrs() []ma.Multiaddr {
	base := g.h.Addrs()
	b := make([]ma.Multiaddr, len(base))
	for i, a := range base {
		m, err := ma.NewMultiaddr(fmt.Sprintf("%s/p2p/%s", a, g.h.ID()))
		if err != nil {
			panic(err)
		}
		b[i] = m
	}
	return b
}

// Shutdown stops the relay node.
func (g *GossipRelayNode) Shutdown() {
	close(g.done)
}

// ParseMultiaddrSlice parses a list of addresses into multiaddrs
func ParseMultiaddrSlice(peers []string) ([]ma.Multiaddr, error) {
	out := make([]ma.Multiaddr, len(peers))
	for i, peer := range peers {
		m, err := ma.NewMultiaddr(peer)
		if err != nil {
			return nil, fmt.Errorf("parsing multiaddr\"%s\": %w", peer, err)
		}
		out[i] = m
	}
	return out, nil
}

func (g *GossipRelayNode) background(w client2.Watcher) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for {
		results := w.Watch(ctx)
	LOOP:
		for {
			select {
			case res, ok := <-results:
				if !ok {
					g.l.Warnw("", "relay_node", "watch channel closed")
					break LOOP
				}

				rd, ok := res.(*client2.RandomData)
				if !ok {
					g.l.Errorw("", "relay_node", "unexpected client result type")
					continue
				}

				randB, err := proto.Marshal(&drand.PublicRandResponse{
					Round:             res.GetRound(),
					Signature:         res.GetSignature(),
					PreviousSignature: rd.GetPreviousSignature(),
					Randomness:        res.GetRandomness(),
				})
				if err != nil {
					g.l.Errorw("", "relay_node", "err marshaling", "err", err)
					continue
				}

				g.l.Debugw("publishing message",
					"relay_node", "publish",
					"round", res.GetRound(),
					"time.Now", time.Now().Unix(),
				)

				err = g.t.Publish(ctx, randB)
				if err != nil {
					g.l.Errorw("", "relay_node", "err publishing on pubsub", "err", err)
					continue
				}

				g.l.Infow("", "relay_node", "Published randomness on pubsub", "round", res.GetRound())
			case <-g.done:
				return
			}
		}
		time.Sleep(time.Second)
	}
}
