package client

import (
	"context"
	"time"

	"github.com/drand/drand/v2/common"
	"github.com/drand/drand/v2/common/chain"
	"github.com/drand/go-clients/drand"
)

const emptyClientStringerValue = "EmptyClient"

// EmptyClientWithInfo makes a client that returns the given info but no randomness
func EmptyClientWithInfo(info *chain.Info) drand.Client {
	return &emptyClient{info}
}

type emptyClient struct {
	i *chain.Info
}

func (m *emptyClient) String() string {
	return emptyClientStringerValue
}

func (m *emptyClient) Info(_ context.Context) (*chain.Info, error) {
	return m.i, nil
}

func (m *emptyClient) RoundAt(t time.Time) uint64 {
	return common.CurrentRound(t.Unix(), m.i.Period, m.i.GenesisTime)
}

func (m *emptyClient) Get(_ context.Context, _ uint64) (drand.Result, error) {
	return nil, drand.ErrEmptyClientUnsupportedGet
}

func (m *emptyClient) Watch(_ context.Context) <-chan drand.Result {
	ch := make(chan drand.Result, 1)
	close(ch)
	return ch
}

func (m *emptyClient) Close() error {
	return nil
}
