package mock

import (
	"context"
	"github.com/drand/drand/protobuf/drand"
	"net"
	"sync"

	grpcmiddleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpcrecovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	grpcprometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
)

var state sync.Mutex

type Listener interface {
	Start()
	Stop(ctx context.Context)
	Addr() string
}

// Service holds all functionalities that a drand node should implement
type Service interface {
	drand.PublicServer
	drand.ControlServer
	drand.ProtocolServer
	drand.Interceptors
	drand.DKGControlServer

	EmitRand(bool)
}

// NewGRPCListenerForPrivate creates a new listener for the Public and Protocol APIs over GRPC.
func NewGRPCListenerForPrivate(_ context.Context, bindingAddr string, s Service, opts ...grpc.ServerOption) (Listener, error) {
	lis, err := net.Listen("tcp", bindingAddr)
	if err != nil {
		return nil, err
	}

	opts = append(opts,
		grpc.StreamInterceptor(
			grpcmiddleware.ChainStreamServer(
				otelgrpc.StreamServerInterceptor(),
				grpcprometheus.StreamServerInterceptor,
				grpcrecovery.StreamServerInterceptor(), // TODO (dlsniper): This turns panics into grpc errors. Do we want that?
			),
		),
		grpc.UnaryInterceptor(
			grpcmiddleware.ChainUnaryServer(
				otelgrpc.UnaryServerInterceptor(),
				grpcprometheus.UnaryServerInterceptor,
				grpcrecovery.UnaryServerInterceptor(), // TODO (dlsniper): This turns panics into grpc errors. Do we want that?
			),
		),
	)

	grpcServer := grpc.NewServer(opts...)
	g := &grpcListener{
		Service:    s,
		grpcServer: grpcServer,
		lis:        lis,
	}

	state.Lock()
	defer state.Unlock()

	return g, nil
}

type grpcListener struct {
	Service
	grpcServer *grpc.Server
	lis        net.Listener
}

func (g *grpcListener) Addr() string {
	return g.lis.Addr().String()
}

func (g *grpcListener) Start() {
	go func() {
		_ = g.grpcServer.Serve(g.lis)
	}()
}

func (g *grpcListener) Stop(_ context.Context) {
	g.grpcServer.Stop()
	_ = g.lis.Close()
}
