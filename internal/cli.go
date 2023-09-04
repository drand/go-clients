package drand

import (
	"fmt"
	"sync"

	"github.com/urfave/cli/v2"

	"github.com/drand/drand/common"
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

var nodeFlag = &cli.StringFlag{
	Name:    "nodes",
	Usage:   "Contact the nodes at the given list of whitespace-separated addresses which have to be present in group.toml.",
	EnvVars: []string{"DRAND_NODES"},
}

var roundFlag = &cli.IntFlag{
	Name: "round",
	Usage: "Request the public randomness generated at round num. If the drand beacon does not have the requested value," +
		" it returns an error. If not specified, the current randomness is returned.",
	EnvVars: []string{"DRAND_ROUND"},
}

var hashOnly = &cli.BoolFlag{
	Name:    "hash",
	Usage:   "Only print the hash of the group file",
	EnvVars: []string{"DRAND_HASH"},
}

// TODO (DLSNIPER): This is a duplicate of the hashInfoReq. Should these be merged into a single flag?
var hashInfoNoReq = &cli.StringFlag{
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
			"drand node.\n",
		Subcommands: []*cli.Command{
			{
				Name: "public",
				Usage: "Get the latest public randomness from the drand " +
					"beacon and verify it against the collective public key " +
					"as specified in group.toml. Only one node is contacted by " +
					"default. This command attempts to connect to the drand " +
					"beacon via TLS and falls back to plaintext communication " +
					"if the contacted node has not activated TLS in which case " +
					"it prints a warning.\n",
				Flags:  toArray(roundFlag, nodeFlag),
				Action: getPublicRandomness,
			},
			{
				Name:      "chain-info",
				Usage:     "Get the binding chain information that this node participates to",
				ArgsUsage: "`ADDRESS1` `ADDRESS2` ... provides the addresses of the node to try to contact to.",
				Flags:     toArray(hashOnly, hashInfoNoReq),
				Action:    getChainInfo,
			},
		},
	},
}

// CLI runs the drand app
func CLI() *cli.App {
	version := common.GetAppVersion()

	app := cli.NewApp()
	app.Name = "drand"

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
	return nil
}

func getChainInfo(c *cli.Context) error {
	return nil
}

func schemesCmd(c *cli.Context) error {
	return nil
}
