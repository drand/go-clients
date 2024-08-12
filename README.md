## go-clients is a Go client library and a set of useful binaries for the drand ecosystem

This repo contains most notably:
 - Go code for interfacing with the drand networks through both HTTP and Gossipsub
 - a client CLI tool to fetch and verify drand beacons from the various available sources in your terminal
 - a gossipsub relay to relay drand beacons on gossipsub

# Migration from drand/drand

Prior to drand V2 release, the drand client code lived in the drand/drand repo. Since its V2 release, the drand daemon code aims at being more minimalistic and having as little dependencies as possible.
Most notably this meant removing the libp2p code that was exclusively used for Gossip relays and Gossip client, and also trimming down the amount of HTTP-related code.

From now one, this repo is meant to provide the Go Client code to interact with drand, query drand beacons and verify them through either the HTTP or the Gossip relays.

Notice that drand does not provide public GRPC endpoint since ~2020, therefore the GRPC client code has been moved to the internal of the relays (to allow relays to directly interface with a working daemon using GRPC).

There are relatively little changes to the public APIs of the client code and simply using the `drand/go-clients/http` packages should be enough.
We recommend using `go-doc` to see the usage documentation and examples.

## Most notable changes from the drand/drand V1 APIs

The `Result` interface now follows the Protobuf getter format:
```
Result.Round() -> Result.GetRound()
Result.Randomness() -> Result.GetRandomness()
Result.Signature() -> Result.GetSignature()
Result.PreviousSignature() -> Result.GetPreviousSignature()
```
meaning `PublicRandResponse` now satisfies directly the `Result` interface.

The HTTP client now returns a concrete type and doesn't need to be cast to a HTTP client to use e.g. `SetUserAgent`.

The client option `WithVerifiedResult` was renamed `WithTrustedResult`, to properly convey its function.

Note also that among other packages you might be using in the `github.com/drand/drand/v2` packages, 
the `crypto.GetSchemeByIDWithDefault` function was renamed `crypto.GetSchemeByID`; 
and the `Beacon` struct now lives in the `github.com/drand/drand/v2/common` package rather than in the `chain` one.

---

### License

This project is licensed using the [Permissive License Stack](https://protocol.ai/blog/announcing-the-permissive-license-stack/) which means that all contributions are available under the most permissive commonly-used licenses, and dependent projects can pick the license that best suits them.

Therefore, the project is dual-licensed under Apache 2.0 and MIT terms:

- Apache License, Version 2.0, ([LICENSE-APACHE](LICENSE-APACHE) or https://www.apache.org/licenses/LICENSE-2.0)
- MIT license ([LICENSE-MIT](LICENSE-MIT) or https://opensource.org/licenses/MIT)
89 
