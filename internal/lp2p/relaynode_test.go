package lp2p

import (
	"context"
	"encoding/hex"
	"errors"
	"github.com/drand/drand/common/key"
	"github.com/drand/drand/crypto"
	"github.com/stretchr/testify/require"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/drand/drand-cli/client"
	"github.com/drand/drand-cli/client/test/result/mock"
	"github.com/drand/drand/common/chain"
	client2 "github.com/drand/drand/common/client"
	"github.com/drand/drand/common/testlogger"
)

type mockClient struct {
	chainInfo *chain.Info
	watchF    func(context.Context) <-chan client2.Result
}

func (c *mockClient) Get(_ context.Context, _ uint64) (client2.Result, error) {
	return nil, errors.New("unsupported")
}

func (c *mockClient) Watch(ctx context.Context) <-chan client2.Result {
	return c.watchF(ctx)
}

func (c *mockClient) Info(_ context.Context) (*chain.Info, error) {
	return c.chainInfo, nil
}

func (c *mockClient) RoundAt(_ time.Time) uint64 {
	return 0
}

func (c *mockClient) Close() error {
	return nil
}

// toRandomDataChain converts the mock results into a chain of client.RandomData
// objects. Note that you do not get back the first result.
func toRandomDataChain(results ...mock.Result) []client.RandomData {
	var randomness []client.RandomData
	prevSig := results[0].GetSignature()
	for i := 1; i < len(results); i++ {
		randomness = append(randomness, client.RandomData{
			Rnd:               results[i].GetRound(),
			Random:            results[i].GetRandomness(),
			Sig:               results[i].GetSignature(),
			PreviousSignature: prevSig,
		})
		prevSig = results[i].GetSignature()
	}
	return randomness
}

func TestWatchRetryOnClose(t *testing.T) {
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	pair, err := key.NewKeyPair("fakeChainInfo.test:1234", sch)
	require.NoError(t, err)

	chainInfo := &chain.Info{
		Period:      time.Second,
		GenesisTime: time.Now().Unix(),
		PublicKey:   pair.Public.Key,
	}

	results := toRandomDataChain(
		mock.NewMockResult(0),
		mock.NewMockResult(1),
		mock.NewMockResult(2),
		mock.NewMockResult(3),
	)
	wg := sync.WaitGroup{}
	wg.Add(len(results))

	// return a channel that writes one result then closes
	watchF := func(context.Context) <-chan client2.Result {
		ch := make(chan client2.Result, 1)
		if len(results) > 0 {
			res := results[0]
			results = results[1:]
			ch <- &res
			wg.Done()
		}
		close(ch)
		return ch
	}

	c := &mockClient{chainInfo, watchF}

	td := t.TempDir()
	lg := testlogger.New(t)
	gr, err := NewGossipRelayNode(lg, &GossipRelayConfig{
		ChainHash:    hex.EncodeToString(chainInfo.Hash()),
		Addr:         "/ip4/0.0.0.0/tcp/0",
		DataDir:      td,
		IdentityPath: path.Join(td, "identity.key"),
		Client:       c,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer gr.Shutdown()
	wg.Wait()

	// even though the watch channel closed, it should have been re-opened by
	// the client multiple times until no results remain.
	if len(results) != 0 {
		t.Fatal("random data items waiting to be consumed", len(results))
	}
}
