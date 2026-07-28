<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
## Table of Contents

- [Drand Pubsub Relay](#drand-pubsub-relay)
  - [Install](#install)
  - [Usage](#usage)
    - [Relay gRPC](#relay-grpc)
    - [Relay HTTP](#relay-http)
    - [Relay Gossipsub](#relay-gossipsub)
    - [Other options](#other-options)
      - [Bootstrap peers](#bootstrap-peers)
      - [Failover](#failover)
      - [Configuring the libp2p pubsub node](#configuring-the-libp2p-pubsub-node)
    - [Usage from a golang drand client](#usage-from-a-golang-drand-client)
      - [With Chain Info](#with-chain-info)
      - [With Known Chain Hash](#with-known-chain-hash)
      - [Insecurely](#insecurely)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

# Drand Pubsub Relay

A program that relays drand randomness rounds over libp2p pubsub (gossipsub) from a gRPC, HTTP, or gossipsub source (labeled as _drand gossipsub relay_ in this diagram):

```
              +-------------------------------+
              |                               |
              |         drand server          |
              |                               |
              +-------------------------------+
              |  gRPC API   |--|   HTTP API   |
              +------^-+------------^-+-------+
                     | |            | |
                     | |            | |
                     | |            | |
                  +--+-v------------+-v---+
                  | drand gossipsub relay |
                  +----------+------------+
                             |
                             |
Publish topic=/drand/pubsub/v0.0.0/<chain-hash> data={randomness}
                             |
                             |
                 +-----------v--------------+
                 |                          |
                 | libp2p gossipsub network |
                 |                          |
                 +--+--------------------+--+
                    |                    |
                    |                    |
        Subscribe topic=/drand/pubsub/v0.0.0/<chain-hash>
                    |                    |
                    |                    |
   +----------------v--------+   +-------v-----------------+
   | drand client WithPubsub |   | drand client WithPubsub |
   +-------------------------+   +-------------------------+
```

## Install

```sh
# Clone this repo
git clone https://github.com/drand/go-clients.git
cd go-clients
# Build the executable
make drand-relay-gossip
# Outputs a `drand-relay-gossip` executable to the current directory.
```

## Usage

In general, you _should_ specify either a `-hash-list` or `-group-conf-list` flag in order for your client to validate the randomness it receives is from the correct chain.

_Note_: You can provide multiple values to both `-hash-list` and`-group-conf-list` flags to support multiple beacons.

### Relay gRPC

```sh
drand-relay-gossip run -grpc-connect=127.0.0.1:3000 \
                       -cert=/path/to/grpc-drand-cert
```

If you do not have gRPC transport credentials, you can use the `-insecure` flag:

```sh
drand-relay-gossip run -grpc-connect=127.0.0.1:3000 \
                       -insecure
```

Or, with a hashlist:
```shell
 drand-relay-gossip run -grpc-connect=127.0.0.1:3000 \
                       -insecure \
                       -hash-list=6093f9e4320c285ac4aab50ba821cd5678ec7c5015d3d9d11ef89e2a99741e83,dbd506d6ef76e5f386f41c651dcb808c5bcbd75471cc4eafa3f4df7ad4e4c493
```

### Relay HTTP

The gossip relay can also relay directly from an HTTP API. You can specify multiple endpoints to enable failover.

```sh
drand-relay-gossip run -url=https://api.drand.sh \
                       -url=https://api2.drand.sh \
                       -hash-list=dbd506d6ef76e5f386f41c651dcb808c5bcbd75471cc4eafa3f4df7ad4e4c493
```

### Relay Gossipsub

The gossip relay can also relay directly from _other_ gossip relays. You can specify multiple peers to directly connect with. In this case, a group configuration file must be specified since there's no way to retrieve chain information over pubsub.

```sh
drand-relay-gossip run -relay=/ip4/127.0.0.1/tcp/44544/p2p/QmPeerID0 \
                       -relay=/ip4/127.0.0.1/tcp/44545/p2p/QmPeerID1 \
                       -group-conf-list=/home/user/.drand/groups/drand_group.toml
```

Alternatively, you can provide URL(s) of HTTP API(s) that can be contacted to retrieve chain information. In this case we must provide the chain `-hash` to verify the information we retrieve is for the chain we expect (or provide the `-insecure` flag):

```sh
drand-relay-gossip run -relay=/ip4/127.0.0.1/tcp/44544/p2p/QmPeerID0 \
                       -relay=/ip4/127.0.0.1/tcp/44545/p2p/QmPeerID1 \
                       -url=http://127.0.0.1:3002 \
                       -hash-list=6093f9e4320c285ac4aab50ba821cd5678ec7c5015d3d9d11ef89e2a99741e83
```

If you want to verify multiple networks, you can provide the `-hash-list` flag, e.g.:

```shell
drand-relay-gossip run -relay=/ip4/127.0.0.1/tcp/44544/p2p/QmPeerID0 \
                       -relay=/ip4/127.0.0.1/tcp/44545/p2p/QmPeerID1 \
                       -url=http://127.0.0.1:3002 \
                       -hash-list=dbd506d6ef76e5f386f41c651dcb808c5bcbd75471cc4eafa3f4df7ad4e4c493,8990e7a9aaed2ffed73dbd7092123d6f289930540d7651336225dc172e51b2ce
```

### Other options

#### Bootstrap peers

If there is a set of peers the gossip relay should connect with and stay connected to then the `-peer-with` flag can be used to specify one or more peer multiaddrs for this purpose.

#### Failover

The `-url` flag provides the URL(s) of alternative HTTP API endpoints that may be able to provide randomness in the event of a failure of the gRPC connection/libp2p pubsub network. Each randomness round is raced with the HTTP endpoints when it becomes available such that if gRPC or pubsub take too long to deliver the round it'll be provided over HTTP e.g.

```sh
drand-relay-gossip run -grpc-connect=127.0.0.1:3000 \
                       -insecure \
                       -url=http://127.0.0.1:3102
```

```sh
drand-relay-gossip run -relay=/ip4/127.0.0.1/tcp/44544/p2p/QmPeerID0 \
                       -relay=/ip4/127.0.0.1/tcp/44545/p2p/QmPeerID1 \
                       -hash-list=6093f9e4320c285ac4aab50ba821cd5678ec7c5015d3d9d11ef89e2a99741e83 \
                       -url=http://127.0.0.1:3102
```

#### Configuring the libp2p pubsub node

Starting a relay will spawn a libp2p pubsub node listening on `/ip4/0.0.0.0/tcp/44544` by default. Use the `-listen` flag to change. To effectively relay drand randomness, your node must be publicly accessible on the network.

If not specified a libp2p identity will be generated and stored in an `identity.key` file in the current working directory. Use the `-identity` flag to override the location.

### Usage from a golang drand client

#### With Chain Info

```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/drand/drand/v2/common/chain"
	"github.com/drand/drand/v2/common/log"
	"github.com/drand/go-clients/client"
	p2pClient "github.com/drand/go-clients/client/lp2p"
)

const (
	// listenAddr is the multiaddr the local libp2p node should listen on.
	listenAddr = "/ip4/0.0.0.0/tcp/4453"
	// chainInfoPath is the path to the chain information (in JSON format), as
	// served by the `/{chain-hash}/info` endpoint of a drand HTTP API.
	chainInfoPath = "/home/user/.drand/chain-info.json"
)

// relayP2PAddrs are the p2p multiaddrs of the drand gossipsub relay nodes to connect to.
var relayP2PAddrs = []string{
	"/dnsaddr/api.drand.sh",
	"/dnsaddr/api2.drand.sh",
	"/dnsaddr/api3.drand.sh",
}

func main() {
	ctx := context.Background()
	l := log.DefaultLogger()

	// Create libp2p pubsub. The local libp2p host is returned as well, so that
	// it can be closed once you are done with it.
	ps, h, err := p2pClient.NewPubsub(ctx, listenAddr, relayP2PAddrs)
	if err != nil {
		l.Panicw("while creating new p2pClient.NewPubsub", "err", err)
	}
	defer h.Close()

	// Read the chain info from a JSON file
	f, err := os.Open(chainInfoPath)
	if err != nil {
		l.Panicw("while opening the chain info file", "err", err)
	}
	defer f.Close()

	info, err := chain.InfoFromJSON(f)
	if err != nil {
		l.Panicw("while parsing the chain info", "err", err)
	}

	c, err := client.New(
		client.WithLogger(l),
		p2pClient.WithPubsub(ps),
		client.WithChainInfo(info),
	)
	if err != nil {
		l.Panicw("while creating a new client", "err", err)
	}

	for res := range c.Watch(ctx) {
		fmt.Printf("round=%v randomness=%x\n", res.GetRound(), res.GetRandomness())
	}
}
```

#### With Known Chain Hash

You do not need to know the full group info to use the pubsub client if you know the chain hash and an HTTP endpoint then you can request the chain info from the HTTP endpoint, verifying it with the known chain hash:

```go
package main

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/drand/drand/v2/common/log"
	"github.com/drand/go-clients/client"
	"github.com/drand/go-clients/client/http"
	gclient "github.com/drand/go-clients/client/lp2p"
)

const (
	// listenAddr is the multiaddr the local libp2p node should listen on.
	listenAddr = "/ip4/0.0.0.0/tcp/4453"
	// chainHash is a hash of the group chain information.
	chainHash = "52db9ba70e0cc0f6eaf7803dd07447a1f5477735fd3f661792ba94600c84e971"
	// httpRelayURL is the URL of a drand HTTP API endpoint.
	httpRelayURL = "https://api.drand.sh"
)

// relayP2PAddrs are the p2p multiaddrs of the drand gossipsub relay nodes to connect to.
var relayP2PAddrs = []string{
	"/dnsaddr/api.drand.sh",
	"/dnsaddr/api2.drand.sh",
	"/dnsaddr/api3.drand.sh",
}

func main() {
	ctx := context.Background()
	lg := log.New(nil, log.InfoLevel, true)

	// Create libp2p pubsub. The host is returned so that it can be closed once done.
	ps, h, err := gclient.NewPubsub(ctx, listenAddr, relayP2PAddrs)
	if err != nil {
		lg.Panicw("while creating new gclient.NewPubsub", "err", err)
	}
	defer h.Close()

	// Chain hash is used to verify endpoints
	hash, err := hex.DecodeString(chainHash)
	if err != nil {
		lg.Panicw("while decoding chain hash", "err", err)
	}

	c, err := client.New(
		client.WithLogger(lg),
		gclient.WithPubsub(ps),
		client.WithChainHash(hash),
		client.From(http.ForURLs(ctx, lg, []string{httpRelayURL}, hash)...),
	)
	if err != nil {
		lg.Panicw("while creating a new client", "err", err)
	}

	for res := range c.Watch(ctx) {
		fmt.Printf("round=%v randomness=%x\n", res.GetRound(), res.GetRandomness())
	}
}
```
