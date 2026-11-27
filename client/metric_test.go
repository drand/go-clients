package client_test

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/drand/drand/v2/crypto"
	"github.com/drand/go-clients/client"
	clientMock "github.com/drand/go-clients/client/mock"
	"github.com/drand/go-clients/client/test/result/mock"
	"github.com/drand/go-clients/drand"
)

func TestWithPrometheusReportsWatchLatency(t *testing.T) {
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	info, results := mock.VerifiableResults(2, sch)

	watcher := &clientMock.Client{
		WatchF: func(ctx context.Context) <-chan drand.Result {
			ch := make(chan drand.Result)
			go func() {
				defer close(ch)
				for i := range results {
					select {
					case ch <- &results[i]:
					case <-ctx.Done():
						return
					}
				}
				<-ctx.Done()
			}()
			return ch
		},
	}

	registry := prometheus.NewRegistry()
	c, err := client.New(
		client.From(watcher, clientMock.ClientWithInfo(info)),
		client.WithChainInfo(info),
		client.WithPrometheus(prometheus.WrapRegistererWith(prometheus.Labels{"client_id": "test"}, registry)),
	)
	require.NoError(t, err)
	defer c.Close()

	require.Eventually(t, func() bool {
		families, err := registry.Gather()
		require.NoError(t, err)
		for _, family := range families {
			if family.GetName() != "client_watch_latency" {
				continue
			}
			metric := family.GetMetric()[0]
			require.Equal(t, "client_id", metric.GetLabel()[0].GetName())
			require.Equal(t, "test", metric.GetLabel()[0].GetValue())
			return metric.GetGauge().GetValue() != 0
		}
		return false
	}, 10*time.Second, 50*time.Millisecond, "client_watch_latency was never set")
}
