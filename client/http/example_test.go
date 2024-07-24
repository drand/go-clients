package http_test

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/drand/drand/v2/crypto"
	client2 "github.com/drand/go-clients/client"
	"github.com/drand/go-clients/client/http"
)

func Example_http_New() {
	chainhash, err := hex.DecodeString("52db9ba70e0cc0f6eaf7803dd07447a1f5477735fd3f661792ba94600c84e971")
	if err != nil {
		// we recommend to handle errors as you wish rather than panicking
		panic(err)
	}

	client, err := http.New(context.Background(), nil, "http://api.drand.sh", chainhash, nil)
	if err != nil {
		panic(err)
	}

	result, err := client.Get(context.Background(), 1234)
	if err != nil {
		panic(err)
	}

	info, err := client.Info(context.Background())
	if err != nil {
		panic(err)
	}

	scheme, err := crypto.SchemeFromName(info.GetSchemeName())
	if err != nil {
		panic(err)
	}

	// make sure to verify the beacons when using the raw http client without a verifying client
	err = scheme.VerifyBeacon(&client2.RandomData{
		Rnd: result.GetRound(),
		Sig: result.GetSignature(),
	}, info.PublicKey)
	if err != nil {
		panic(err)
	}

	fmt.Printf("got beacon: round=%d; randomness=%x\n", result.GetRound(), result.GetRandomness())

	//output: got beacon: round=1234; randomness=9ead58abb451d8f521338c43ba5595610642a0c07d0e9babeaae6a98787629de
}
