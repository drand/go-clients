/*
Package http provides a drand client implementation that uses drand's HTTP API.

The HTTP client uses drand's JSON HTTP API
(https://drand.love/developer/http-api/) to fetch randomness. Watching is
implemented by polling the endpoint at the expected round time.

Example:

	package main

	import (
		"context"
		"encoding/hex"

		"github.com/drand/drand/v2/client"
		"github.com/drand/drand/v2/client/http"
		"github.com/drand/drand/v2/common/log"
	)

	var urls = []string{
		"https://api.drand.sh",
		"https://drand.cloudflare.com",
	}

	var chainHash, _ = hex.DecodeString("8990e7a9aaed2ffed73dbd7092123d6f289930540d7651336225dc172e51b2ce")

	func main() {
		ctx := context.Background()
		lg := log.New(nil, log.DebugLevel, true)

		c, err := client.New(ctx, lg,
			client.From(http.ForURLs(ctx, lg, urls, chainHash)...),
			client.WithChainHash(chainHash),
		)
	}

The "ForURLs" helper creates multiple HTTP clients from a list of
URLs. Alternatively you can use the "New" or "NewWithInfo" constructor to
create clients.

Tip: Provide multiple URLs to enable failover and speed optimized URL
selection.
*/
package http
