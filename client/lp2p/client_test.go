package lp2p

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/consul/sdk/freeport"
	clock "github.com/jonboulle/clockwork"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"

	"github.com/drand/drand/v2/test/mock"
	"github.com/drand/go-clients/drand"
	"github.com/drand/go-clients/internal/grpc"

	"github.com/drand/drand/v2/common/chain"
	"github.com/drand/drand/v2/common/log"
	"github.com/drand/drand/v2/crypto"
	dhttp "github.com/drand/go-clients/client/http"
	httpmock "github.com/drand/go-clients/client/test/http/mock"
	"github.com/drand/go-clients/internal/lp2p"
)

func TestGRPCClientTestFunc(t *testing.T) {
	lg := log.New(nil, log.DebugLevel, true)
	// start mock drand node
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)

	clk := clock.NewFakeClockAt(time.Now())

	grpcLis, svc := mock.NewMockGRPCPublicServer(t, lg, "127.0.0.1:0", false, sch, clk)
	grpcAddr := grpcLis.Addr()
	go grpcLis.Start()
	defer grpcLis.Stop(context.Background())

	dataDir := t.TempDir()
	identityDir := t.TempDir()

	infoProto, err := svc.ChainInfo(context.Background(), nil)
	require.NoError(t, err)

	info, err := chain.InfoFromProto(infoProto)
	require.NoError(t, err)

	// start mock relay-node
	grpcClient, err := grpc.New(grpcAddr, true, []byte(""))
	require.NoError(t, err)

	cfg := &lp2p.GossipRelayConfig{
		ChainHash:    info.HashString(),
		PeerWith:     nil,
		Addr:         "/ip4/127.0.0.1/tcp/0",
		DataDir:      dataDir,
		IdentityPath: path.Join(identityDir, "identity.key"),
		Client:       grpcClient,
	}
	g, err := lp2p.NewGossipRelayNode(lg, cfg)
	require.NoError(t, err, "gossip relay node")

	defer g.Shutdown()

	// start client
	c, err := newTestClient(t, g.Multiaddrs(), info, clk)
	require.NoError(t, err)
	defer func() {
		err := c.Close()
		require.NoError(t, err)
	}()

	// test client
	ctx, cancel := context.WithCancel(context.Background())
	ch := c.Watch(ctx)

	baseRound := uint64(1969)

	mockService := svc.(mock.Service)
	// pub sub polls every 200ms
	wait := 250 * time.Millisecond
	for i := uint64(0); i < 3; i++ {
		time.Sleep(wait)
		mockService.EmitRand(false)
		t.Logf("round %d emitted\n", baseRound+i)

		select {
		case r, ok := <-ch:
			require.True(t, ok, "expected randomness, watch outer channel was closed instead")
			t.Logf("received round %d\n", r.GetRound())
			require.Equal(t, baseRound+i, r.GetRound())
		// the period of the mock servers is 1 second
		case <-time.After(5 * time.Second):
			t.Fatal("timeout.")
		}
	}

	time.Sleep(wait)
	mockService.EmitRand(true)
	cancel()

	drain(t, ch, 5*time.Second)
}

func drain(t *testing.T, ch <-chan drand.Result, timeout time.Duration) {
	t.Helper()
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-time.After(timeout):
			t.Fatal("timed out closing channel.")
		}
	}
}

func TestHTTPClientTestFunc(t *testing.T) {
	ctx := context.Background()
	lg := log.New(nil, log.DebugLevel, true)
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)

	clk := clock.NewFakeClockAt(time.Now())

	addr, chainInfo, stop, emit := httpmock.NewMockHTTPPublicServer(t, false, sch, clk)
	defer stop()

	dataDir := t.TempDir()
	identityDir := t.TempDir()

	httpClient, err := dhttp.New(ctx, lg, "http://"+addr, chainInfo.Hash(), http.DefaultTransport)
	require.NoError(t, err)

	cfg := &lp2p.GossipRelayConfig{
		ChainHash:    chainInfo.HashString(),
		PeerWith:     nil,
		Addr:         "/ip4/127.0.0.1/tcp/" + strconv.Itoa(freeport.GetOne(t)),
		DataDir:      dataDir,
		IdentityPath: path.Join(identityDir, "identity.key"),
		Client:       httpClient,
	}
	g, err := lp2p.NewGossipRelayNode(lg, cfg)
	if err != nil {
		t.Fatalf("gossip relay node (%v)", err)
	}
	defer g.Shutdown()

	c, err := newTestClient(t, g.Multiaddrs(), chainInfo, clk)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	ctx, cancel := context.WithCancel(ctx)
	emit(false)
	ch := c.Watch(ctx)
	for i := 0; i < 3; i++ {
		// pub sub polls every 200ms, but the other http one polls every period
		time.Sleep(1250 * time.Millisecond)
		emit(false)
		select {
		case r, ok := <-ch:
			if !ok {
				t.Fatal("expected randomness")
			} else {
				t.Log("received randomness", r.GetRound())
			}
		case <-time.After(8 * time.Second):
			t.Fatal("timeout.")
		}
	}
	emit(true)
	cancel()
	drain(t, ch, 5*time.Second)
}

func newTestClient(t *testing.T, relayMultiaddr []ma.Multiaddr, info *chain.Info, _ clock.Clock) (*Client, error) {
	identityDir := t.TempDir()

	lg := log.New(nil, log.DebugLevel, true)
	priv, err := lp2p.LoadOrCreatePrivKey(path.Join(identityDir, "identity.key"), lg)
	if err != nil {
		return nil, err
	}
	h, ps, err := lp2p.ConstructHost(priv, "/ip4/0.0.0.0/tcp/0", relayMultiaddr, lg)
	if err != nil {
		return nil, err
	}
	relayPeerID, err := peerIDFromMultiaddr(relayMultiaddr[0])
	if err != nil {
		return nil, err
	}
	err = waitForConnection(h, relayPeerID, time.Minute)
	if err != nil {
		return nil, err
	}
	c, err := NewWithPubsub(lg, ps, info, nil)
	if err != nil {
		return nil, err
	}
	c.SetLog(lg)
	return c, nil
}

func peerIDFromMultiaddr(addr ma.Multiaddr) (peer.ID, error) {
	ai, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return "", err
	}
	return ai.ID, nil
}

func waitForConnection(h host.Host, id peer.ID, timeout time.Duration) error {
	t := time.NewTimer(timeout)
	for {
		if len(h.Network().ConnsToPeer(id)) > 0 {
			t.Stop()
			return nil
		}
		select {
		case <-t.C:
			return fmt.Errorf("timed out waiting to be connected the relay @ %v", id)
		default:
		}
		time.Sleep(time.Millisecond * 100)
	}
}
