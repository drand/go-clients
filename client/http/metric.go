package http

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/drand/go-clients/drand"

	"github.com/drand/drand/v2/common"
	"github.com/drand/go-clients/internal/metrics"
)

// MeasureHeartbeats periodically tracks latency observed on a set of HTTP clients
func MeasureHeartbeats(ctx context.Context, c []drand.Client) *HealthMetrics {
	m := &HealthMetrics{
		next:    0,
		clients: c,
	}
	if len(c) > 0 {
		go m.startObserve(ctx)
	}
	return m
}

// HealthMetrics is a measurement task around HTTP clients
type HealthMetrics struct {
	next    int
	clients []drand.Client
}

// HeartbeatInterval is the duration between liveness heartbeats sent to an HTTP API.
const HeartbeatInterval = 10 * time.Second

func (c *HealthMetrics) startObserve(ctx context.Context) {
	// we check all clients within HeartbeatInterval
	interval := time.Duration(int64(HeartbeatInterval) / int64(len(c.clients)))
	for {
		// check if ctx is Done
		if ctx.Err() != nil {
			return
		}
		time.Sleep(interval)
		n := c.next % len(c.clients)

		httpClient, ok := c.clients[n].(*httpClient)
		if !ok {
			c.next++
			continue
		}

		result, err := c.clients[n].Get(ctx, c.clients[n].RoundAt(time.Now())+1)
		if err != nil {
			metrics.ClientHTTPHeartbeatFailure.With(prometheus.Labels{"http_address": httpClient.root}).Inc()
			continue
		}

		metrics.ClientHTTPHeartbeatSuccess.With(prometheus.Labels{"http_address": httpClient.root}).Inc()

		// compute the latency metric
		actual := time.Now().UnixNano()
		expected := common.TimeOfRound(httpClient.chainInfo.Period, httpClient.chainInfo.GenesisTime, result.GetRound()) * 1e9
		// the labels of the gauge vec must already be set at the registerer level
		metrics.ClientHTTPHeartbeatLatency.
			With(prometheus.Labels{"http_address": httpClient.root}).
			Set(float64(actual-expected) / float64(time.Millisecond))
		c.next++
	}
}
