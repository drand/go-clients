package client_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/drand/go-clients/drand"

	"github.com/drand/drand/v2/common/log"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/go-clients/client"
	clientMock "github.com/drand/go-clients/client/mock"
	"github.com/drand/go-clients/client/test/result/mock"
)

func mockClientWithVerifiableResults(_ context.Context, t *testing.T, _ log.Logger, n int, strictRounds bool) (drand.Client, []mock.Result) {
	t.Helper()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)

	info, results := mock.VerifiableResults(n, sch)
	mc := clientMock.Client{Results: results, StrictRounds: strictRounds, OptionalInfo: info}

	var c drand.Client

	c, err = client.Wrap(
		[]drand.Client{clientMock.ClientWithInfo(info), &mc},
		client.WithChainInfo(info),
		client.WithTrustedResult(&results[0]),
		client.WithFullChainVerification(),
	)
	require.NoError(t, err)

	return c, results
}

func TestVerify(t *testing.T) {
	VerifyFuncTest(t, 3, 1)
}

func TestVerifyWithOldVerifiedResult(t *testing.T) {
	VerifyFuncTest(t, 5, 4)
}

func VerifyFuncTest(t *testing.T, clients, upTo int) {
	ctx := context.Background()
	l := log.New(nil, log.DebugLevel, true)
	c, results := mockClientWithVerifiableResults(ctx, t, l, clients, true)

	res, err := c.Get(context.Background(), results[upTo].GetRound())
	require.NoError(t, err)

	if res.GetRound() != results[upTo].GetRound() {
		t.Fatal("expected to get result.", results[upTo].GetRound(), res.GetRound(), fmt.Sprintf("%v", c))
	}
}

func TestGetWithRoundMismatch(t *testing.T) {
	ctx := context.Background()
	l := log.New(nil, log.DebugLevel, true)
	c, results := mockClientWithVerifiableResults(ctx, t, l, 5, false)
	for i := 1; i < len(results); i++ {
		results[i] = results[0]
	}

	_, err := c.Get(context.Background(), 3)
	require.ErrorContains(t, err, "round mismatch (malicious relay): 1 != 3")
}

// TestVerifyFullChainWithoutTrustedResult exercises full chain verification
// with no point of trust configured, so the verifier has to walk the chain from
// the genesis seed. The other tests in this file all pass WithTrustedResult,
// which takes a different path and hid an off-by-one in the walk.
func TestVerifyFullChainWithoutTrustedResult(t *testing.T) {
	// the off-by-one only shows up on chained schemes, since unchained ones
	// ignore the previous signature entirely
	sch, err := crypto.GetSchemeByID(crypto.DefaultSchemeID)
	require.NoError(t, err)

	info, results := mock.VerifiableResults(5, sch)
	mc := clientMock.Client{Results: results, StrictRounds: true, OptionalInfo: info}

	c, err := client.Wrap(
		[]drand.Client{clientMock.ClientWithInfo(info), &mc},
		client.WithChainInfo(info),
		client.WithFullChainVerification(),
	)
	require.NoError(t, err)

	// round 2 is the first round whose previous signature has to come from the
	// chain itself rather than from the genesis seed
	for _, round := range []uint64{2, 3, 5} {
		res, err := c.Get(context.Background(), round)
		require.NoError(t, err, "round %d should verify against the genesis seed", round)
		require.Equal(t, round, res.GetRound())
	}
}
