package client_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	client2 "github.com/drand/drand-cli/client"
	clientMock "github.com/drand/drand-cli/client/mock"
	"github.com/drand/drand-cli/client/test/result/mock"
	"github.com/drand/drand/common/client"
	"github.com/drand/drand/common/log"
	"github.com/drand/drand/common/testlogger"
	"github.com/drand/drand/crypto"
)

func mockClientWithVerifiableResults(ctx context.Context, t *testing.T, l log.Logger, n int, strictRounds bool) (client.Client, []mock.Result) {
	t.Helper()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)

	info, results := mock.VerifiableResults(n, sch)
	mc := clientMock.Client{Results: results, StrictRounds: strictRounds, OptionalInfo: info}

	var c client.Client

	c, err = client2.Wrap(
		ctx,
		l,
		[]client.Client{clientMock.ClientWithInfo(info), &mc},
		client2.WithChainInfo(info),
		client2.WithVerifiedResult(&results[0]),
		client2.WithFullChainVerification(),
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
	l := testlogger.New(t)
	c, results := mockClientWithVerifiableResults(ctx, t, l, clients, true)

	res, err := c.Get(context.Background(), results[upTo].GetRound())
	require.NoError(t, err)

	if res.GetRound() != results[upTo].GetRound() {
		t.Fatal("expected to get result.", results[upTo].GetRound(), res.GetRound(), fmt.Sprintf("%v", c))
	}
}

func TestGetWithRoundMismatch(t *testing.T) {
	ctx := context.Background()
	l := testlogger.New(t)
	c, results := mockClientWithVerifiableResults(ctx, t, l, 5, false)
	for i := 1; i < len(results); i++ {
		results[i] = results[0]
	}

	_, err := c.Get(context.Background(), 3)
	require.ErrorContains(t, err, "round mismatch (malicious relay): 1 != 3")
}
