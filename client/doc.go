/*
Package client provides transport-agnostic logic to retrieve and verify
randomness from drand, including retry, validation, caching and
optimization features.

The "From" option allows you to specify clients that work over particular
transports. HTTP, gRPC and libp2p PubSub clients are provided as
subpackages https://pkg.go.dev/github.com/drand/go-clients/internal/client/http,
https://pkg.go.dev/github.com/drand/go-clients/internal/client/grpc and
https://pkg.go.dev/github.com/drand/go-clients/internal/lp2p/clientlp2p/client
respectively. Note that you are not restricted to just one client. You can use
multiple clients of the same type or of different types. The base client will
periodically "speed test" it's clients, failover, cache results and aggregate
calls to "Watch" to reduce requests.

WARNING: When using the client you should use the "WithChainHash" or
"WithChainInfo" option in order for your client to validate the randomness it
receives is from the correct chain. You may use the "Insecurely" option to
bypass this validation but it is not recommended.

In an application that uses the drand client, the following options are likely
to be needed/customized:

	WithCacheSize()
		should be set to something sensible for your application.

	WithVerifiedResult()
	WithFullChainVerification()
		both should be set for increased security if you have
		persistent state and expect to be following the chain.

	WithAutoWatch()
		will pre-load new results as they become available adding them
		to the cache for speedy retreival when you need them.

	WithPrometheus()
		enables metrics reporting on speed and performance to a
		provided prometheus registry.
*/
package client
