package http

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	clock "github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"

	"github.com/drand/drand-cli/client"
	"github.com/drand/drand-cli/client/test/http/mock"
	"github.com/drand/drand/common/testlogger"
	"github.com/drand/drand/crypto"
)

func TestHTTPClient(t *testing.T) {
	ctx := context.Background()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	clk := clock.NewFakeClockAt(time.Now())
	addr, chainInfo, cancel, _ := mock.NewMockHTTPPublicServer(t, true, sch, clk)
	defer cancel()

	err = IsServerReady(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}

	l := testlogger.New(t)
	httpClient, err := New(ctx, l, "http://"+addr, chainInfo.Hash(), http.DefaultTransport)
	if err != nil {
		t.Fatal(err)
	}

	ctx1, cancel1 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel1()
	result, err := httpClient.Get(ctx1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.GetRandomness()) == 0 {
		t.Fatal("no randomness provided")
	}
	full, ok := (result).(*client.RandomData)
	if !ok {
		t.Fatal("Should be able to restore concrete type")
	}
	if len(full.Sig) == 0 {
		t.Fatal("no signature provided")
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	if _, err := httpClient.Get(ctx2, full.Rnd+1); err != nil {
		t.Fatalf("http client should not perform verification of results. err: %s", err)
	}
	_ = httpClient.Close()
}

func TestHTTPGetLatest(t *testing.T) {
	ctx := context.Background()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	clk := clock.NewFakeClockAt(time.Now())
	addr, chainInfo, cancel, _ := mock.NewMockHTTPPublicServer(t, false, sch, clk)
	defer cancel()

	err = IsServerReady(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}

	l := testlogger.New(t)
	httpClient, err := New(ctx, l, "http://"+addr, chainInfo.Hash(), http.DefaultTransport)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel = context.WithTimeout(ctx, time.Second)
	defer cancel()
	r0, err := httpClient.Get(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel = context.WithTimeout(ctx, time.Second)
	defer cancel()
	r1, err := httpClient.Get(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}

	if r1.GetRound() != r0.GetRound()+1 {
		t.Fatal("expected round progression")
	}
	_ = httpClient.Close()
}

func TestForURLsCreation(t *testing.T) {
	ctx := context.Background()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	clk := clock.NewFakeClockAt(time.Now())
	addr, chainInfo, cancel, _ := mock.NewMockHTTPPublicServer(t, false, sch, clk)
	defer cancel()

	err = IsServerReady(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}

	l := testlogger.New(t)
	clients := ForURLs(ctx, l, []string{"http://invalid.domain/", "http://" + addr}, chainInfo.Hash())
	if len(clients) != 2 {
		t.Fatal("expect both urls returned")
	}
	_ = clients[0].Close()
	_ = clients[1].Close()
}

func TestHTTPWatch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	ctx := context.Background()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	clk := clock.NewFakeClockAt(time.Now())
	addr, chainInfo, cancel, _ := mock.NewMockHTTPPublicServer(t, false, sch, clk)
	defer cancel()

	err = IsServerReady(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}

	l := testlogger.New(t)
	httpClient, err := New(ctx, l, "http://"+addr, chainInfo.Hash(), http.DefaultTransport)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result := httpClient.Watch(ctx)
	first, ok := <-result
	if !ok {
		t.Fatal("should get a result from watching")
	}
	if len(first.GetRandomness()) == 0 {
		t.Fatal("should get randomness from watching")
	}

	for range result {
	}
	_ = httpClient.Close()
}

func TestHTTPClientClose(t *testing.T) {
	ctx := context.Background()
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	clk := clock.NewFakeClockAt(time.Now())
	addr, chainInfo, cancel, _ := mock.NewMockHTTPPublicServer(t, false, sch, clk)
	defer cancel()

	err = IsServerReady(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}

	l := testlogger.New(t)
	httpClient, err := New(ctx, l, "http://"+addr, chainInfo.Hash(), http.DefaultTransport)
	if err != nil {
		t.Fatal(err)
	}
	result, err := httpClient.Get(context.Background(), 1969)
	if err != nil {
		t.Fatal(err)
	}
	if result.GetRound() != 1969 {
		t.Fatal("unexpected round.")
	}

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		for range httpClient.Watch(context.Background()) {
		}
		wg.Done()
	}()

	err = httpClient.Close()
	if err != nil {
		t.Fatal(err)
	}

	_, err = httpClient.Get(context.Background(), 0)
	if !errors.Is(err, errClientClosed) {
		t.Fatal("unexpected error from closed client", err)
	}

	wg.Wait() // wait for the watch to close
}

//nolint:funlen
//func TestHTTPRelay(t *testing.T) {
//	lg := testlogger.New(t)
//	ctx := log.ToContext(context.Background(), lg)
//	ctx, cancel := context.WithCancel(ctx)
//	defer cancel()
//
//	clk := clock.NewFakeClockAt(time.Now())
//	c, _ := withClient(t, clk)
//
//	handler, err := dhandler.New(ctx, "")
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	info, err := c.Info(ctx)
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	handler.RegisterNewBeaconHandler(c, info.HashString())
//
//	listener, err := net.Listen("tcp", "127.0.0.1:0")
//	if err != nil {
//		t.Fatal(err)
//	}
//	server := http.Server{Handler: handler.GetHTTPHandler()}
//	go func() { _ = server.Serve(listener) }()
//	defer func() { _ = server.Shutdown(ctx) }()
//
//	err = nhttp.IsServerReady(ctx, listener.Addr().String())
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	getChains := fmt.Sprintf("http://%s/chains", listener.Addr().String())
//	resp := getWithCtx(ctx, getChains, t)
//	if resp.StatusCode != http.StatusOK {
//		t.Error("expected http status code 200")
//	}
//	var chains []string
//	require.NoError(t, json.NewDecoder(resp.Body).Decode(&chains))
//	require.NoError(t, resp.Body.Close())
//
//	if len(chains) != 1 {
//		t.Error("expected chain hash qty not valid")
//	}
//	if chains[0] != info.HashString() {
//		t.Error("expected chain hash not valid")
//	}
//
//	getChain := fmt.Sprintf("http://%s/%s/info", listener.Addr().String(), info.HashString())
//	resp = getWithCtx(ctx, getChain, t)
//	cip := new(drand.ChainInfoPacket)
//	require.NoError(t, json.NewDecoder(resp.Body).Decode(cip))
//	require.NotNil(t, cip.Hash)
//	require.NotNil(t, cip.PublicKey)
//	require.NoError(t, resp.Body.Close())
//
//	// Test exported interfaces.
//	u := fmt.Sprintf("http://%s/%s/public/2", listener.Addr().String(), info.HashString())
//	resp = getWithCtx(ctx, u, t)
//	body := make(map[string]interface{})
//	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
//	require.NoError(t, resp.Body.Close())
//
//	if _, ok := body["signature"]; !ok {
//		t.Fatal("expected signature in random response.")
//	}
//
//	u = fmt.Sprintf("http://%s/%s/public/latest", listener.Addr().String(), info.HashString())
//	resp, err = http.Get(u)
//	if err != nil {
//		t.Fatal(err)
//	}
//	body = make(map[string]interface{})
//
//	if err = json.NewDecoder(resp.Body).Decode(&body); err != nil {
//		t.Fatal(err)
//	}
//	require.NoError(t, resp.Body.Close())
//
//	if _, ok := body["round"]; !ok {
//		t.Fatal("expected signature in latest response.")
//	}
//}
