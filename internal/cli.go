package drand

import (
	"fmt"
	"sync"

	"github.com/urfave/cli/v2"

	"github.com/drand/drand/v2/common"
)

// Automatically set through -ldflags
// Example: go install -ldflags "-X main.buildDate=$(date -u +%d/%m/%Y@%H:%M:%S) -X main.gitCommit=$(git rev-parse HEAD)"
var (
	gitCommit = "none"
	buildDate = "unknown"
)

var SetVersionPrinter sync.Once

var verboseFlag = &cli.BoolFlag{
	Name:    "verbose",
	Usage:   "If set, verbosity is at the debug level",
	EnvVars: []string{"DRAND_VERBOSE"},
}

var relayFlag = &cli.StringSliceFlag{
	Name:    "relays",
	Usage:   "Contact the HTTP relay at the given URL address. Can be specified multiple times to try multiple relays.",
	EnvVars: []string{"DRAND_HTTP_RELAY"},
}

var roundFlag = &cli.IntFlag{
	Name: "round",
	Usage: "Request the public randomness generated at round num. If the requested value doesn't exist yet," +
		" it returns an error. If not specified or 0, the latest beacon is returned.",
	EnvVars: []string{"DRAND_ROUND"},
}

var hashInfoFlag = &cli.StringFlag{
	Name:    "chain-hash",
	Usage:   "The hash of the chain info",
	EnvVars: []string{"DRAND_CHAIN_HASH"},
}

var jsonFlag = &cli.BoolFlag{
	Name:    "json",
	Usage:   "Set the output as json format",
	EnvVars: []string{"DRAND_JSON"},
}

var beaconIDFlag = &cli.StringFlag{
	Name:    "id",
	Usage:   "Indicates the id of the beacon chain you're interested in",
	Value:   "",
	EnvVars: []string{"DRAND_ID"},
}

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
				Flags:  toArray(roundFlag, relayFlag, jsonFlag),
				Action: getPublicRandomness,
			},
			{
				Name:      "chain-info",
				Usage:     "Get the binding chain information that this node participates to",
				ArgsUsage: "`ADDRESS1` `ADDRESS2` ... provides the addresses of the node to try to contact to.",
				Flags:     toArray(hashInfoFlag, relayFlag, jsonFlag),
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

	app.ExitErrHandler = func(context *cli.Context, err error) {
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
	// we need to copy the underlying flags to avoid races
	verbFlag := *verboseFlag
	app.Flags = toArray(&verbFlag)
	return app
}

func isVerbose(c *cli.Context) bool {
	return c.IsSet(verboseFlag.Name)
}

func toArray(flags ...cli.Flag) []cli.Flag {
	return flags
}

// TODO
func getPublicRandomness(c *cli.Context) error {
	fmt.Println("currently unimplemented")
	return nil
}

func getChainInfo(c *cli.Context) error {
	fmt.Println("currently unimplemented")
	return nil
}

func schemesCmd(c *cli.Context) error {
	fmt.Println("currently unimplemented")
	return nil
}
