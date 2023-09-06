package net

// HTTPClient is an optional extension to the protocol client relaying of HTTP over the GRPC connection.
import (
	"context"
	"net/http"
)

type Peer interface {
	Address() string
}

// it is currently used for relaying metrics between group members.
type HTTPClient interface {
	HandleHTTP(ctx context.Context, p Peer) (http.Handler, error)
}
