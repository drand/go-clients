package drand

import (
	"fmt"
	"log"
	"strconv"
	"sync"

	json "github.com/nikkolasg/hexjson"
	"github.com/urfave/cli/v2"

	client "github.com/drand/drand/v2/common/client"

	"github.com/drand/drand-cli/internal/lib"
	"github.com/drand/drand/v2/common"
)

// Automatically set through -ldflags
// Example: go install -ldflags "-X main.buildDate=$(date -u +%d/%m/%Y@%H:%M:%S) -X main.gitCommit=$(git rev-parse HEAD)"
var (
	gitCommit = "none"
	buildDate = "unknown"
)

var SetVersionPrinter sync.Once

var appCommands = []*cli.Command{
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
				Flags:     toArray(lib.URLFlag, lib.JSONFlag, lib.InsecureFlag, lib.HashListFlag, lib.VerboseFlag),
				ArgsUsage: "--url url1 --url url2 ROUND... uses the first working relay to query round number ROUND",
				Action:    getPublicRandomness,
			},
			{
				Name:      "chain-info",
				Usage:     "Get beacon information",
				ArgsUsage: "--url url1 --url url2 ... uses the first working relay",
				Flags:     toArray(lib.URLFlag, lib.JSONFlag, lib.InsecureFlag, lib.HashListFlag, lib.VerboseFlag),
				Action:    getChainInfo,
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
	return flags
}

func instantiateClient(cctx *cli.Context) (client.Client, error) {
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
	json.NewEncoder(cctx.App.Writer).Encode(round)
	return nil
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
