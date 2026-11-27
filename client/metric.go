package client

import (
	"context"
	"time"

	"github.com/drand/drand/v2/common"
	"github.com/drand/drand/v2/common/chain"

	"github.com/drand/go-clients/drand"
	"github.com/drand/go-clients/internal/metrics"
)

// newWatchLatencyMetricClient wraps base and reports in
// metrics.ClientWatchLatency how late each watched round arrives.
func newWatchLatencyMetricClient(base drand.Client, info *chain.Info) drand.Client {
	ctx, cancel := context.WithCancel(context.Background())
	c := &watchLatencyMetricClient{
		Client:    base,
		chainInfo: info,
		cancel:    cancel,
	}
	go c.observe(ctx)
	return c
}

type watchLatencyMetricClient struct {
	drand.Client
	chainInfo *chain.Info
	cancel    context.CancelFunc
}

func (c *watchLatencyMetricClient) observe(ctx context.Context) {
	for result := range c.Watch(ctx) {
		expected := time.Unix(common.TimeOfRound(c.chainInfo.Period, c.chainInfo.GenesisTime, result.GetRound()), 0)
		metrics.ClientWatchLatency.Set(float64(time.Since(expected)) / float64(time.Millisecond))
	}
}

// Close stops the latency observation and closes the wrapped client.
func (c *watchLatencyMetricClient) Close() error {
	c.cancel()
	return c.Client.Close()
}
