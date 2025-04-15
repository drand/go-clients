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
