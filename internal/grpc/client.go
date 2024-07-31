package grpc

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"time"

	grpcProm "github.com/grpc-ecosystem/go-grpc-prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	grpcInsec "google.golang.org/grpc/credentials/insecure"

	"github.com/drand/go-clients/drand"

	"github.com/drand/drand/v2/crypto"

	commonutils "github.com/drand/drand/v2/common"
	"github.com/drand/drand/v2/common/chain"
	"github.com/drand/drand/v2/common/log"
	proto "github.com/drand/drand/v2/protobuf/drand"
	"github.com/drand/go-clients/client"
)

const grpcDefaultTimeout = 5 * time.Second

type grpcClient struct {
	address   string
	chainHash []byte
	client    proto.PublicClient
	conn      *grpc.ClientConn
	l         log.Logger
}

// New creates a drand client backed by a GRPC connection.
func New(address string, insecure bool, chainHash []byte) (drand.Client, error) {
	var opts []grpc.DialOption
	if insecure {
		opts = append(opts, grpc.WithTransportCredentials(grpcInsec.NewCredentials()))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})))
	}
	opts = append(opts,
		grpc.WithUnaryInterceptor(grpcProm.UnaryClientInterceptor),
		grpc.WithStreamInterceptor(grpcProm.StreamClientInterceptor),
	)
	conn, err := grpc.NewClient(address, opts...)
	if err != nil {
		return nil, err
	}

	return &grpcClient{address, chainHash, proto.NewPublicClient(conn), conn, log.DefaultLogger()}, nil
}

func asRD(r *proto.PublicRandResponse) *client.RandomData {
	return &client.RandomData{
		Rnd:               r.GetRound(),
		Random:            crypto.RandomnessFromSignature(r.GetSignature()),
		Sig:               r.GetSignature(),
		PreviousSignature: r.GetPreviousSignature(),
	}
}

// String returns the name of this client.
func (g *grpcClient) String() string {
	return fmt.Sprintf("GRPC(%q)", g.address)
}

// Get returns a the randomness at `round` or an error.
func (g *grpcClient) Get(ctx context.Context, round uint64) (drand.Result, error) {
	curr, err := g.client.PublicRand(ctx, &proto.PublicRandRequest{Round: round, Metadata: g.getMetadata()})
	if err != nil {
		return nil, err
	}
	if curr == nil {
		return nil, errors.New("no received randomness - unexpected gPRC response")
	}

	return asRD(curr), nil
}

// Watch returns new randomness as it becomes available.
func (g *grpcClient) Watch(ctx context.Context) <-chan drand.Result {
	stream, err := g.client.PublicRandStream(ctx, &proto.PublicRandRequest{Round: 0, Metadata: g.getMetadata()})
	ch := make(chan drand.Result, 1)
	if err != nil {
		close(ch)
		return ch
	}
	go g.translate(stream, ch)
	return ch
}

// Info returns information about the chain.
func (g *grpcClient) Info(ctx context.Context) (*chain.Info, error) {
	p, err := g.client.ChainInfo(ctx, &proto.ChainInfoRequest{Metadata: g.getMetadata()})
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, errors.New("no received group - unexpected gPRC response")
	}
	return chain.InfoFromProto(p)
}

func (g *grpcClient) translate(stream proto.Public_PublicRandStreamClient, out chan<- drand.Result) {
	defer close(out)
	for {
		next, err := stream.Recv()
		if err != nil || stream.Context().Err() != nil {
			if stream.Context().Err() == nil {
				g.l.Warnw("", "grpc_client", "public rand stream", "err", err)
			}
			return
		}
		out <- asRD(next)
	}
}

func (g *grpcClient) getMetadata() *proto.Metadata {
	return &proto.Metadata{ChainHash: g.chainHash}
}

func (g *grpcClient) RoundAt(t time.Time) uint64 {
	ctx, cancel := context.WithTimeout(context.Background(), grpcDefaultTimeout)
	defer cancel()

	info, err := g.client.ChainInfo(ctx, &proto.ChainInfoRequest{Metadata: g.getMetadata()})
	if err != nil {
		return 0
	}
	return commonutils.CurrentRound(t.Unix(), time.Second*time.Duration(info.Period), info.GenesisTime)
}

// SetLog configures the client log output
func (g *grpcClient) SetLog(l log.Logger) {
	g.l = l
}

// Close tears down the gRPC connection and all underlying connections.
func (g *grpcClient) Close() error {
	return g.conn.Close()
}
