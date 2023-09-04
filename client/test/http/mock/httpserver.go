package mock

import (
	"context"
	clock "github.com/jonboulle/clockwork"
	"testing"

	chainCommon "github.com/drand/drand/common/chain"
	"github.com/drand/drand/crypto"
)

// NewMockHTTPPublicServer creates a mock drand HTTP server for testing.
func NewMockHTTPPublicServer(t *testing.T, badSecondRound bool, sch *crypto.Scheme, clk clock.Clock) (string, *chainCommon.Info, context.CancelFunc, func(bool)) {
	t.Helper()
	//ctx := context.Background()
	panic("non-implemented")
}
