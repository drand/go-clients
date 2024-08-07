package client_test

import (
	"context"
	"errors"
	"testing"
	"time"

	clock "github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"

	"github.com/drand/drand/v2/common/key"
	"github.com/drand/drand/v2/common/log"
	clientMock "github.com/drand/go-clients/client/mock"
	"github.com/drand/go-clients/client/test/result/mock"
	"github.com/drand/go-clients/drand"

	"github.com/drand/drand/v2/common/chain"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/go-clients/client"
	"github.com/drand/go-clients/client/http"
	httpmock "github.com/drand/go-clients/client/test/http/mock"
)

func TestClientConstraints(t *testing.T) {
	if _, e := client.New(); e == nil {
		t.Fatal("client can't be created without root of trust")
	}

	if _, e := client.New(client.WithChainHash([]byte{0})); e == nil {
		t.Fatal("Client needs URLs if only a chain hash is specified")
	}

	if _, e := client.New(client.From(clientMock.ClientWithResults(0, 5))); e == nil {
		t.Fatal("Client needs root of trust unless insecure specified explicitly")
	}

	c := clientMock.ClientWithResults(0, 5)
	// As we will run is insecurely, we will set chain info so client can fetch it
	c.OptionalInfo = fakeChainInfo(t)

	if _, e := client.New(client.From(c), client.Insecurely()); e != nil {
		t.Fatal(e)
	}
}

func TestClientMultiple(t *testing.T) {
	ctx := context.Background()
	lg := log.New(nil, log.DebugLevel, true)
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	clk := clock.NewFakeClockAt(time.Now())

	addr1, chainInfo, cancel, _ := httpmock.NewMockHTTPPublicServer(t, false, sch, clk)
	defer cancel()

	addr2, chaininfo2, cancel2, _ := httpmock.NewMockHTTPPublicServer(t, false, sch, clk)
	defer cancel2()

	t.Log("created mockhttppublicserver", "addr", addr1, "chaininfo", chainInfo)
	t.Log("created mockhttppublicserver", "addr", addr2, "chaininfo", chaininfo2)

	// TODO: review this, are we really expecting this to work when the two servers aren't serving the same chainhash?
	httpClients := http.ForURLs(ctx, lg, []string{"http://" + addr1, "http://" + addr2}, chainInfo.Hash())
	if len(httpClients) == 0 {
		t.Error("http clients is empty")
		return
	}

	c, e := client.New(client.From(httpClients...), client.WithChainHash(chainInfo.Hash()))

	if e != nil {
		t.Fatal(e)
	}
	r, e := c.Get(ctx, 0)
	if e != nil {
		t.Fatal(e)
	}
	if r.GetRound() <= 0 {
		t.Fatal("expected valid client")
	}
	_ = c.Close()
}

func TestClientWithChainInfo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	ctx := context.Background()
	chainInfo := fakeChainInfo(t)
	lg := log.New(nil, log.DebugLevel, true)
	hc, err := http.NewWithInfo(lg, "http://nxdomain.local/", chainInfo, nil)
	require.NoError(t, err)
	c, err := client.New(client.WithChainInfo(chainInfo), client.From(hc))
	if err != nil {
		t.Fatal("existing group creation shouldn't do additional validaiton.")
	}
	_, err = c.Get(ctx, 0)
	if err == nil {
		t.Fatal("bad urls should clearly not provide randomness.")
	}
	_ = c.Close()
}

func TestClientCache(t *testing.T) {
	ctx := context.Background()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	clk := clock.NewFakeClockAt(time.Now())
	addr1, chainInfo, cancel, _ := httpmock.NewMockHTTPPublicServer(t, false, sch, clk)
	defer cancel()

	lg := log.New(nil, log.DebugLevel, true)
	httpClients := http.ForURLs(ctx, lg, []string{"http://" + addr1}, chainInfo.Hash())
	if len(httpClients) == 0 {
		t.Error("http clients is empty")
		return
	}

	c, e := client.New(
		client.From(httpClients...),
		client.WithChainHash(chainInfo.Hash()),
		client.WithCacheSize(1),
	)

	if e != nil {
		t.Fatal(e)
	}
	r0, e := c.Get(ctx, 0)
	if e != nil {
		t.Fatal(e)
	}
	cancel()
	_, e = c.Get(ctx, r0.GetRound())
	if e != nil {
		t.Fatal(e)
	}

	_, e = c.Get(ctx, 4)
	if e == nil {
		t.Fatal("non-cached results should fail.")
	}
	_ = c.Close()
}

func TestClientWithoutCache(t *testing.T) {
	ctx := context.Background()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	clk := clock.NewFakeClockAt(time.Now())
	addr1, chainInfo, cancel, _ := httpmock.NewMockHTTPPublicServer(t, false, sch, clk)
	defer cancel()

	lg := log.New(nil, log.DebugLevel, true)
	httpClients := http.ForURLs(ctx, lg, []string{"http://" + addr1}, chainInfo.Hash())
	if len(httpClients) == 0 {
		t.Error("http clients is empty")
		return
	}

	c, err := client.New(
		client.From(httpClients...),
		client.WithChainHash(chainInfo.Hash()),
		client.WithCacheSize(0))

	require.NoError(t, err)

	_, err = c.Get(ctx, 0)
	require.NoError(t, err)
	cancel()
	_, err = c.Get(ctx, 0)
	require.Error(t, err)

	_ = c.Close()
}

func TestClientWithWatcher(t *testing.T) {
	ctx := context.Background()
	lg := log.New(nil, log.DebugLevel, true)
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	info, results := mock.VerifiableResults(2, sch)

	ch := make(chan drand.Result, len(results))
	for i := range results {
		ch <- &results[i]
	}
	close(ch)

	watcherCtor := func(l log.Logger, chainInfo *chain.Info, _ client.Cache) (client.Watcher, error) {
		return &clientMock.Client{WatchCh: ch}, nil
	}

	var c drand.Client
	c, err = client.New(client.WithLogger(lg),
		client.WithChainInfo(info),
		client.WithWatcher(watcherCtor),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	w := c.Watch(ctx)

	for i := 0; i < len(results); i++ {
		r := <-w
		compareResults(t, &results[i], r)
	}
	require.NoError(t, c.Close())
}

func TestClientWithWatcherCtorError(t *testing.T) {
	watcherErr := errors.New("boom")
	watcherCtor := func(l log.Logger, chainInfo *chain.Info, _ client.Cache) (client.Watcher, error) {
		return nil, watcherErr
	}

	// constructor should return error returned by watcherCtor
	_, err := client.New(
		client.WithChainInfo(fakeChainInfo(t)),
		client.WithWatcher(watcherCtor),
	)
	if !errors.Is(err, watcherErr) {
		t.Fatal(err)
	}
}

func TestClientChainHashOverrideError(t *testing.T) {
	lg := log.New(nil, log.DebugLevel, true)
	chainInfo := fakeChainInfo(t)
	_, err := client.Wrap(
		[]drand.Client{client.EmptyClientWithInfo(chainInfo)},
		client.WithChainInfo(chainInfo),
		client.WithChainHash(fakeChainInfo(t).Hash()),
		client.WithLogger(lg),
	)
	if err == nil {
		t.Fatal("expected error, received no error")
	}
	if err.Error() != "refusing to override group with non-matching hash" {
		t.Fatal(err)
	}
}

func TestClientChainInfoOverrideError(t *testing.T) {
	lg := log.New(nil, log.DebugLevel, true)
	chainInfo := fakeChainInfo(t)
	_, err := client.Wrap(
		[]drand.Client{client.EmptyClientWithInfo(chainInfo)},
		client.WithChainHash(chainInfo.Hash()),
		client.WithChainInfo(fakeChainInfo(t)),
		client.WithLogger(lg),
	)
	if err == nil {
		t.Fatal("expected error, received no error")
	}
	if err.Error() != "refusing to override hash with non-matching group" {
		t.Fatal(err)
	}
}

func TestClientAutoWatch(t *testing.T) {
	ctx := context.Background()
	lg := log.New(nil, log.DebugLevel, true)
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	clk := clock.NewFakeClockAt(time.Now())
	addr1, chainInfo, cancel, _ := httpmock.NewMockHTTPPublicServer(t, false, sch, clk)
	defer cancel()

	httpClient := http.ForURLs(ctx, lg, []string{"http://" + addr1}, chainInfo.Hash())
	if len(httpClient) == 0 {
		t.Error("http clients is empty")
		return
	}

	r1, _ := httpClient[0].Get(ctx, 1)
	r2, _ := httpClient[0].Get(ctx, 2)
	results := []drand.Result{r1, r2}

	ch := make(chan drand.Result, len(results))
	for i := range results {
		ch <- results[i]
	}
	close(ch)

	watcherCtor := func(l log.Logger, chainInfo *chain.Info, _ client.Cache) (client.Watcher, error) {
		return &clientMock.Client{WatchCh: ch}, nil
	}

	var c drand.Client
	c, err = client.New(
		client.From(clientMock.ClientWithInfo(chainInfo)),
		client.WithChainHash(chainInfo.Hash()),
		client.WithWatcher(watcherCtor),
		client.WithAutoWatch(),
	)

	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(chainInfo.Period)
	cancel()
	r, err := c.Get(ctx, results[0].GetRound())
	if err != nil {
		t.Fatal(err)
	}
	compareResults(t, r, results[0])
	_ = c.Close()
}

func TestClientAutoWatchRetry(t *testing.T) {
	ctx := context.Background()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)

	info, results := mock.VerifiableResults(5, sch)
	resC := make(chan drand.Result)
	defer close(resC)

	// done is closed after all resuls have been written to resC
	done := make(chan struct{})

	// Returns a channel that yields the verifiable results above
	watchF := func(ctx context.Context) <-chan drand.Result {
		go func() {
			for i := 0; i < len(results); i++ {
				select {
				case resC <- &results[i]:
				case <-ctx.Done():
					return
				}
			}
			<-time.After(time.Second)
			close(done)
		}()
		return resC
	}

	var failer clientMock.Client
	failer = clientMock.Client{
		WatchF: func(ctx context.Context) <-chan drand.Result {
			// First call returns a closed channel
			ch := make(chan drand.Result)
			close(ch)
			// Second call returns a channel that writes results
			failer.WatchF = watchF
			return ch
		},
	}

	var c drand.Client
	c, err = client.New(
		client.From(&failer, clientMock.ClientWithInfo(info)),
		client.WithChainInfo(info),
		client.WithAutoWatch(),
		client.WithAutoWatchRetry(time.Second),
		client.WithCacheSize(len(results)),
	)

	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// Wait for all the results to be consumed by the autoWatch
	select {
	case <-done:
	case <-time.After(time.Minute):
		t.Fatal("timed out waiting for results to be consumed")
	}

	// We should be able to retrieve all the results from the cache.
	for i := range results {
		r, err := c.Get(ctx, results[i].GetRound())
		if err != nil {
			t.Fatal(err)
		}
		compareResults(t, &results[i], r)
	}
}

// compareResults asserts that two results are the same.
func compareResults(t *testing.T, expected, actual drand.Result) {
	t.Helper()

	require.NotNil(t, expected)
	require.NotNil(t, actual)
	require.Equal(t, expected.GetRound(), actual.GetRound())
	require.Equal(t, expected.GetRandomness(), actual.GetRandomness())
}

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
