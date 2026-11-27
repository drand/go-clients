package drand

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	json "github.com/nikkolasg/hexjson"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/urfave/cli/v2"

	"github.com/drand/go-clients/client"
	"github.com/drand/go-clients/drand"

	"github.com/drand/drand/v2/common"
	"github.com/drand/go-clients/internal/lib"
)

// Automatically set through -ldflags
// Example: go install -ldflags "-X main.buildDate=$(date -u +%d/%m/%Y@%H:%M:%S) -X main.gitCommit=$(git rev-parse HEAD)"
var (
	gitCommit = "none"
	buildDate = "unknown"
)

var SetVersionPrinter sync.Once

var clientMetricsAddressFlag = &cli.StringFlag{
	Name:  "client-metrics-address",
	Usage: "host:port to serve the client Prometheus metrics on, requires --client-metrics-id",
}

var clientMetricsIDFlag = &cli.StringFlag{
	Name:  "client-metrics-id",
	Usage: "Value of the client_id label set on the client metrics, requires --client-metrics-address",
}

var appCommands = []*cli.Command{
	{
		Name: "watch",
		Usage: "Print new public randomness as it becomes available, " +
			"from the given http-relays and gossip relays.\n",
		Flags:  append(toArray(lib.ClientFlags...), lib.PortFlag, clientMetricsAddressFlag, clientMetricsIDFlag),
		Action: watch,
	},
	{
		Name: "get",
		Usage: "get allows for public information retrieval from a remote " +
			"drand http-relay.\n",
		Subcommands: []*cli.Command{
			{
				Name: "public",
				Usage: "Get the latest public randomness from the drand " +
					"relay and verify it against the collective public key " +
					"as specified in the chain-info.\n",
				Flags: toArray(lib.URLFlag, lib.JSONFlag, lib.InsecureFlag, lib.HashFlag,
					lib.HashListFlag, lib.GroupConfFlag, lib.VerboseFlag),
				ArgsUsage: "--url url1 --url url2 ROUND... uses the first working relay to query round number ROUND",
				Action:    getPublicRandomness,
			},
			{
				Name:      "chain-info",
				Usage:     "Get beacon information",
				ArgsUsage: "--url url1 --url url2 ... uses the first working relay",
				Flags: toArray(lib.URLFlag, lib.JSONFlag, lib.InsecureFlag, lib.HashFlag,
					lib.HashListFlag, lib.GroupConfFlag, lib.VerboseFlag),
				Action: getChainInfo,
			},
		},
	},
}

// CLI runs the drand app
func CLI() *cli.App {
	version := common.GetAppVersion()

	app := cli.NewApp()
	app.Name = "drand-client"

	// See https://cli.urfave.org/v2/examples/bash-completions/#enabling for how to turn on.
	app.EnableBashCompletion = true

	SetVersionPrinter.Do(func() {
		cli.VersionPrinter = func(c *cli.Context) {
			fmt.Fprintf(c.App.Writer, "drand %s (date %v, commit %v)\n", version, buildDate, gitCommit)
		}
	})

	app.ExitErrHandler = func(_ *cli.Context, _ error) {
		// override to prevent default behavior of calling OS.exit(1),
		// when tests expect to be able to run multiple commands.
	}
	app.Version = version.String()
	app.Usage = "distributed randomness service"
	// =====Commands=====
	// we need to copy the underlying commands to avoid races, cli sadly doesn't support concurrent executions well
	appComm := make([]*cli.Command, len(appCommands))
	for i, p := range appCommands {
		if p == nil {
			continue
		}
		v := *p
		appComm[i] = &v
	}
	app.Commands = appComm

	return app
}

func toArray(flags ...cli.Flag) []cli.Flag {
	return append([]cli.Flag(nil), flags...)
}

func instantiateClient(cctx *cli.Context) (drand.Client, error) {
	c, err := lib.Create(cctx, false)
	if err != nil {
		return nil, fmt.Errorf("constructing client: %w", err)
	}

	_, err = c.Info(cctx.Context)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve chain info from relay: %w", err)
	}

	return c, nil
}

func getPublicRandomness(cctx *cli.Context) error {
	c, err := instantiateClient(cctx)
	if err != nil {
		return err
	}
	if cctx.Args().Len() > 1 {
		log.Fatal("please specify a single round as positional argument")
	}

	var r uint64
	if val := cctx.Args().Get(0); val != "" {
		r, err = strconv.ParseUint(val, 10, 64)
		if err != nil {
			return err
		}
	}

	round, err := c.Get(cctx.Context, r)
	if err != nil {
		return err
	}
	return json.NewEncoder(cctx.App.Writer).Encode(round)
}

func watch(cctx *cli.Context) error {
	addr := cctx.String(clientMetricsAddressFlag.Name)
	id := cctx.String(clientMetricsIDFlag.Name)
	if (addr == "") != (id == "") {
		return fmt.Errorf("--%s and --%s must be set together", clientMetricsAddressFlag.Name, clientMetricsIDFlag.Name)
	}

	var opts []client.Option
	if addr != "" {
		registry := prometheus.NewRegistry()
		srv, err := serveMetrics(cctx.Context, addr, registry)
		if err != nil {
			return err
		}
		defer srv.Close()
		opts = append(opts, client.WithPrometheus(
			prometheus.WrapRegistererWith(prometheus.Labels{"client_id": id}, registry)))
	}

	c, err := lib.Create(cctx, addr != "", opts...)
	if err != nil {
		return fmt.Errorf("constructing client: %w", err)
	}
	defer c.Close()

	for r := range c.Watch(cctx.Context) {
		fmt.Fprintf(cctx.App.Writer, "%d\t%x\n", r.GetRound(), r.GetRandomness())
	}
	return nil
}

const metricsTimeout = 10 * time.Second

// serveMetrics serves registry on addr under /metrics until the returned
// server is closed.
func serveMetrics(ctx context.Context, addr string, registry *prometheus.Registry) (*http.Server, error) {
	lis, err := (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listening for metrics on %s: %w", addr, err)
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{Timeout: metricsTimeout}))
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: metricsTimeout,
		ReadTimeout:       metricsTimeout,
		WriteTimeout:      2 * metricsTimeout,
	}
	go func() {
		if err := srv.Serve(lis); !errors.Is(err, http.ErrServerClosed) {
			log.Printf("metrics server stopped: %v", err)
		}
	}()
	return srv, nil
}

func getChainInfo(cctx *cli.Context) error {
	c, err := instantiateClient(cctx)
	if err != nil {
		return err
	}

	info, err := c.Info(cctx.Context)
	if err != nil {
		return err
	}

	return info.ToJSON(cctx.App.Writer, nil)
}
