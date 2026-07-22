/*
Package client provides transport-agnostic logic to retrieve and verify
randomness from drand, including retry, validation, caching and
optimization features.

The "From" option allows you to specify clients that work over particular
transports. HTTP and libp2p PubSub clients are provided as subpackages
https://pkg.go.dev/github.com/drand/go-clients/client/http and
https://pkg.go.dev/github.com/drand/go-clients/client/lp2p respectively.
Note that drand does not expose public gRPC endpoints, so the gRPC client
lives in the internal packages used by the relays. Note that you are not restricted to just one client. You can use
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

	WithTrustedResult()
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
