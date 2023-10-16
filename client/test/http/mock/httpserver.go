package mock

import (
	"context"
	"github.com/drand/drand-cli/client/mock"
	"github.com/drand/drand/common/chain"
	"github.com/drand/drand/common/key"
	"github.com/drand/drand/common/log"
	"github.com/drand/drand/common/testlogger"
	dhttp "github.com/drand/drand/handler/http"
	clock "github.com/jonboulle/clockwork"
	"net"
	"net/http"
	"testing"
	"time"

	chainCommon "github.com/drand/drand/common/chain"
	"github.com/drand/drand/crypto"
)

// NewMockHTTPPublicServer creates a mock drand HTTP server for testing.
func NewMockHTTPPublicServer(t *testing.T, badSecondRound bool, sch *crypto.Scheme, clk clock.Clock) (string, *chainCommon.Info, context.CancelFunc) {
	t.Helper()
	lg := testlogger.New(t)
	ctx := log.ToContext(context.Background(), lg)
	ctx, cancel := context.WithCancel(ctx)

	handler, err := dhttp.New(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

	pair, err := key.NewKeyPair("fakeChainInfo.test:1234", sch)
	if err != nil {
		t.Fatal(err)
	}

	chainInfo := &chain.Info{
		Period:      time.Second,
		GenesisTime: clk.Now().Unix(),
		PublicKey:   pair.Public.Key,
		Scheme:      sch.Name,
	}

	n := uint64(0)
	if badSecondRound {
		n = 149
	}
	client := mock.ClientWithResults(n, 150)
	handler.RegisterNewBeaconHandler(client, chainInfo.HashString())

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	httpServer := http.Server{Handler: handler.GetHTTPHandler(), ReadHeaderTimeout: 3 * time.Second}
	go httpServer.Serve(listener)

	return listener.Addr().String(), chainInfo, func() {
		httpServer.Shutdown(ctx)
		cancel()
	}
}
