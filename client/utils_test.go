package client

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/drand/drand/v2/common/chain"
	"github.com/drand/drand/v2/common/client"
	"github.com/drand/drand/v2/common/key"
	"github.com/drand/drand/v2/crypto"
)

// fakeChainInfo creates a chain info object for use in tests.
func fakeChainInfo(t *testing.T) *chain.Info {
	t.Helper()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	pair, err := key.NewKeyPair("fakeChainInfo.test:1234", sch)
	require.NoError(t, err)

	return &chain.Info{
		Period:      time.Second,
		GenesisTime: time.Now().Unix(),
		PublicKey:   pair.Public.Key,
		Scheme:      sch.Name,
	}
}

func latestResult(t *testing.T, c client.Client) client.Result {
	t.Helper()
	r, err := c.Get(context.Background(), 0)
	if err != nil {
		t.Fatal("getting latest result", err)
	}
	return r
}

// nextResult reads the next result from the channel and fails the test if it closes before a value is read.
func nextResult(t *testing.T, ch <-chan client.Result) client.Result {
	t.Helper()

	select {
	case r, ok := <-ch:
		if !ok {
			t.Fatal("closed before result")
		}
		return r
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for result.")
		return nil
	}
}

// compareResults asserts that two results are the same.
func compareResults(t *testing.T, a, b client.Result) {
	t.Helper()

	if a.GetRound() != b.GetRound() {
		t.Fatal("unexpected result round", a.GetRound(), b.GetRound())
	}
	if !bytes.Equal(a.GetRandomness(), b.GetRandomness()) {
		t.Fatal("unexpected result randomness", a.GetRandomness(), b.GetRandomness())
	}
}
