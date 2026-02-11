package examples_test

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/drand/drand/v2/common/log"
	"github.com/drand/go-clients/client"
	"github.com/drand/go-clients/client/http"
)

var chainHash = "8990e7a9aaed2ffed73dbd7092123d6f289930540d7651336225dc172e51b2ce"

func Example_BasicUsage() {
	lg := log.New(nil, log.DebugLevel, true)

	httpClient, err := http.NewSimpleClient("http://api.drand.sh/", chainHash)
	chb, err := hex.DecodeString(chainHash)

	c, err := client.New(client.From(httpClient), // use a concrete client implementations
		client.WithChainHash(chb),
		client.WithLogger(lg),
	)
	if err != nil {
		panic(err)
	}

	// e.g. use the client to get the latest randomness round:
	r, err := c.Get(context.Background(), 1)
	if err != nil {
		panic(err)
	}

	fmt.Println(r.GetRound())
	//output: 1
}
