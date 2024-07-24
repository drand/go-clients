package lp2p_test

import (
	"bytes"
	"context"
	"fmt"
	"time"

	clock "github.com/jonboulle/clockwork"

	"github.com/drand/drand/v2/common"
	"github.com/drand/drand/v2/common/chain"
	"github.com/drand/drand/v2/common/log"
	gclient "github.com/drand/go-clients/client/lp2p"
)

const (
	// relayP2PAddr is the p2p multiaddr of the drand gossipsub relay node to connect to.
	relayP2PAddr  = "/dnsaddr/api.drand.sh"
	relayP2PAddr2 = "/dnsaddr/api2.drand.sh"
	relayP2PAddr3 = "/dnsaddr/api3.drand.sh"

	// jsonQuicknetInfo, can be hardcoded since these don't change over time
	jsonQuicknetInfo = `{
  "genesis_time": 1692803367,
  "groupHash": "f477d5c89f21a17c863a7f937c6a6d15859414d2be09cd448d4279af331c5d3e",
  "hash": "52db9ba70e0cc0f6eaf7803dd07447a1f5477735fd3f661792ba94600c84e971",
  "metadata": {
    "beaconID": "quicknet"
  },
  "period": 3,
  "public_key": "83cf0f2896adee7eb8b5f01fcad3912212c437e0073e911fb90022d3e760183c8c4b450b6a0a6c3ac6a5776a2d1064510d1fec758c921cc22b0e17e63aaf4bcb5ed66304de9cf809bd274ca73bab4af5a6e9c76a4bc09e76eae8991ef5ece45a",
  "schemeID": "bls-unchained-g1-rfc9380"
}
`
)

func ExampleNewPubsub() {
	ctx := context.Background()

	// /0 to use a random free port
	ps, h, err := gclient.NewPubsub(ctx, "/ip4/0.0.0.0/tcp/0", []string{relayP2PAddr, relayP2PAddr2, relayP2PAddr3})
	if err != nil {
		panic(err)
	}
	defer h.Close()

	info, err := chain.InfoFromJSON(bytes.NewReader([]byte(jsonQuicknetInfo)))
	if err != nil {
		panic(err)
	}

	// NewWithPubSub will automatically register the topic for the chainhash you're interested in
	c, err := gclient.NewWithPubsub(log.DefaultLogger(), ps, info, nil, clock.NewRealClock(), gclient.DefaultBufferSize)
	if err != nil {
		panic(err)
	}

	// This can be slow to "start"
	for res := range c.Watch(context.Background()) {
		expected := common.CurrentRound(time.Now().Unix(), info.Period, info.GenesisTime)
		fmt.Println("correct round:", expected == res.GetRound(), "with", len(res.GetRandomness()), "random bytes")
		// we just waited on the first one as an example
		break
	}

	//output: correct round: true with 32 random bytes
}
