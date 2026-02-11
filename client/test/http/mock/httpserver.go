package mock

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/drand/go-clients/drand"
	clock "github.com/jonboulle/clockwork"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"

	"github.com/drand/drand/v2/common"
	"github.com/drand/drand/v2/common/chain"
	old "github.com/drand/drand/v2/common/client"
	dhttp "github.com/drand/drand/v2/handler/http"
	proto "github.com/drand/drand/v2/protobuf/drand"
	"github.com/drand/drand/v2/test/mock"
	"github.com/drand/go-clients/client"

	"github.com/drand/drand/v2/crypto"
)

// NewMockHTTPPublicServer creates a mock drand HTTP server for testing.
func NewMockHTTPPublicServer(t *testing.T, badSecondRound bool, sch *crypto.Scheme, clk clock.Clock) (string, *chain.Info, context.CancelFunc, func(bool)) {
	t.Helper()

	server := mock.NewMockServer(t, badSecondRound, sch, clk)
	c := Proxy(server)

	ctx, cancel := context.WithCancel(context.Background())

	handler, err := dhttp.New(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

	var chainInfo *chain.Info
	for range 3 {
		protoInfo, err := server.ChainInfo(ctx, &proto.ChainInfoRequest{})
		if err != nil {
			t.Error("MockServer.ChainInfo error:", err)
			time.Sleep(10 * time.Millisecond)
			continue
		}
		chainInfo, err = chain.InfoFromProto(protoInfo)
		if err != nil {
			t.Error("MockServer.InfoFromProto error:", err)
			time.Sleep(10 * time.Millisecond)
			continue
		}

		break
	}
	if chainInfo == nil {
		t.Fatal("could not use server after 3 attempts.")
	}

	t.Log("MockServer.ChainInfo:", chainInfo)

	handler.RegisterDefaultBeaconHandler(handler.RegisterNewBeaconHandler(c, chainInfo.HashString()))

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}

	httpServer := http.Server{Handler: handler.GetHTTPHandler(), ReadHeaderTimeout: 3 * time.Second}
	go httpServer.Serve(listener)

	return listener.Addr().String(), chainInfo, func() {
		httpServer.Shutdown(context.Background())
		cancel()
	}, server.(mock.Service).EmitRand
}

// drandProxy is used as a proxy between a Public service (e.g. the node as a server)
// and a Public Client (the client consumed by the HTTP API)
type drandProxy struct {
	r         proto.PublicServer
	proxyChan chan old.Result
}

// Proxy wraps a server interface into an old client interface so it can be queried
func Proxy(s proto.PublicServer) old.Client {
	return &drandProxy{s, nil}
}

// String returns the name of this proxy.
func (d *drandProxy) String() string {
	return "Proxy"
}

// Get returns randomness at a requested round
func (d *drandProxy) Get(ctx context.Context, round uint64) (old.Result, error) {
	resp, err := d.r.PublicRand(ctx, &proto.PublicRandRequest{Round: round})
	if err != nil {
		return nil, err
	}
	return &client.RandomData{
		Rnd:               resp.GetRound(),
		Random:            crypto.RandomnessFromSignature(resp.GetSignature()),
		Sig:               resp.GetSignature(),
		PreviousSignature: resp.GetPreviousSignature(),
	}, nil
}

// Watch returns new randomness as it becomes available.
func (d *drandProxy) Watch(ctx context.Context) <-chan old.Result {
	proxy := newStreamProxy(ctx)
	go func() {
		err := d.r.PublicRandStream(&proto.PublicRandRequest{}, proxy)
		if err != nil {
			proxy.Close()
		}
	}()

	ch := make(chan old.Result, 1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				close(ch)
				return
			case in := <-proxy.outgoing:
				ch <- old.Result(in)
			}
		}

	}()
	d.proxyChan = ch
	return ch
}

// Info returns the parameters of the chain this client is connected to.
// The public key, when it started, and how frequently it updates.
func (d *drandProxy) Info(ctx context.Context) (*chain.Info, error) {
	info, err := d.r.ChainInfo(ctx, &proto.ChainInfoRequest{})
	if err != nil {
		return nil, err
	}
	return chain.InfoFromProto(info)
}

// RoundAt will return the most recent round of randomness that will be available
// at time for the current client.
func (d *drandProxy) RoundAt(t time.Time) uint64 {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	info, err := d.Info(ctx)
	if err != nil {
		return 0
	}
	return common.CurrentRound(t.Unix(), info.Period, info.GenesisTime)
}

func (d *drandProxy) Close() error {
	return nil
}

// streamProxy directly relays messages of the PublicRandResponse stream.
type streamProxy struct {
	ctx      context.Context
	cancel   context.CancelFunc
	outgoing chan drand.Result
}

func newStreamProxy(ctx context.Context) *streamProxy {
	ctx, cancel := context.WithCancel(ctx)
	s := streamProxy{
		ctx:      ctx,
		cancel:   cancel,
		outgoing: make(chan drand.Result, 1),
	}
	return &s
}

func (s *streamProxy) Send(next *proto.PublicRandResponse) error {
	d := common.Beacon{
		Round:       next.Round,
		Signature:   next.Signature,
		PreviousSig: next.PreviousSignature,
	}
	select {
	case s.outgoing <- &d:
		return nil
	case <-s.ctx.Done():
		close(s.outgoing)
		return s.ctx.Err()
	default:
		return nil
	}
}

func (s *streamProxy) Close() {
	s.cancel()
}

/* implement the grpc stream interface. not used since messages passed directly. */

func (s *streamProxy) SetHeader(metadata.MD) error {
	return nil
}
func (s *streamProxy) SendHeader(metadata.MD) error {
	return nil
}
func (s *streamProxy) SetTrailer(metadata.MD) {}

func (s *streamProxy) Context() context.Context {
	return peer.NewContext(s.ctx, &peer.Peer{Addr: &net.UnixAddr{}})
}
func (s *streamProxy) SendMsg(_ any) error {
	return nil
}
func (s *streamProxy) RecvMsg(_ any) error {
	return nil
}

func (s *streamProxy) Header() (metadata.MD, error) {
	return nil, nil
}

func (s *streamProxy) Trailer() metadata.MD {
	return nil
}
func (s *streamProxy) CloseSend() error {
	return nil
}
