// Package drand is a distributed randomness beacon. It provides periodically an
// unpredictable, bias-resistant, and verifiable random value.
package drand

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/urfave/cli/v2"

	"github.com/drand/drand-cli/internal/core"
	"github.com/drand/drand-cli/internal/core/migration"
	"github.com/drand/drand-cli/internal/fs"
	"github.com/drand/drand-cli/internal/net"
	"github.com/drand/drand/common"
	"github.com/drand/drand/common/key"
	"github.com/drand/drand/common/log"
	"github.com/drand/drand/crypto"
	common2 "github.com/drand/drand/protobuf/common"
	"github.com/drand/drand/protobuf/drand"
)

// Automatically set through -ldflags
// Example: go install -ldflags "-X main.buildDate=$(date -u +%d/%m/%Y@%H:%M:%S) -X main.gitCommit=$(git rev-parse HEAD)"
var (
	gitCommit = "none"
	buildDate = "unknown"
)

var SetVersionPrinter sync.Once

const defaultPort = "8080"

func banner(w io.Writer) {
	version := common.GetAppVersion()
	_, _ = fmt.Fprintf(w, "drand %s (date %v, commit %v)\n", version.String(), buildDate, gitCommit)
}

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
	Usage:   "Indicates the id for the randomness generation process which will be started",
	Value:   "",
	EnvVars: []string{"DRAND_ID"},
}

var listIdsFlag = &cli.BoolFlag{
	Name:    "list-ids",
	Usage:   "Indicates if it only have to list the running beacon ids instead of the statuses.",
	Value:   false,
	EnvVars: []string{"DRAND_LIST_IDS"},
}

var allBeaconsFlag = &cli.BoolFlag{
	Name:    "all",
	Usage:   "Indicates if we have to interact with all beacons chains",
	Value:   false,
	EnvVars: []string{"DRAND_ALL"},
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
				Flags: toArray(roundFlag, nodeFlag),
				Action: func(c *cli.Context) error {
					l := log.New(nil, logLevel(c), logJSON(c)).
						Named("getPublicRandomness")
					return getPublicRandomness(c, l)
				},
			},
			{
				Name:      "chain-info",
				Usage:     "Get the binding chain information that this node participates to",
				ArgsUsage: "`ADDRESS1` `ADDRESS2` ... provides the addresses of the node to try to contact to.",
				Flags:     toArray(hashOnly, hashInfoNoReq),
				Action: func(c *cli.Context) error {
					l := log.New(nil, logLevel(c), logJSON(c)).
						Named("getChainInfo")
					return getChainInfo(c, l)
				},
			},
		},
	},
	{
		Name:  "util",
		Usage: "Multiple commands of utility functions, such as reseting a state, checking the connection of a peer...",
		Subcommands: []*cli.Command{
			{
				Name:  "list-schemes",
				Usage: "List all scheme ids available to use\n",
				Action: func(c *cli.Context) error {
					l := log.New(nil, logLevel(c), logJSON(c)).
						Named("schemesCmd")
					return schemesCmd(c, l)
				},
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
	foldFlag := *folderFlag
	app.Flags = toArray(&verbFlag, &foldFlag)
	app.Before = testWindows
	return app
}

func resetCmd(c *cli.Context, l log.Logger) error {
	conf := contextToConfig(c, l)

	fmt.Fprintf(c.App.Writer, "You are about to delete your local share, group file and generated random beacons. "+
		"Are you sure you wish to perform this operation? [y/N]")
	reader := bufio.NewReader(c.App.Reader)

	answer, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("error reading: %w", err)
	}

	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer != "y" {
		fmt.Fprintf(c.App.Writer, "drand: not reseting the state.")
		return nil
	}

	stores, err := getKeyStores(c, l)
	if err != nil {
		fmt.Fprintf(c.App.Writer, "drand: err reading beacons database: %v\n", err)
		os.Exit(1)
	}

	for beaconID, store := range stores {
		if err := store.Reset(); err != nil {
			fmt.Fprintf(c.App.Writer, "drand: beacon id [%s] - err reseting key store: %v\n", beaconID, err)
			os.Exit(1)
		}

		if err := os.RemoveAll(path.Join(conf.ConfigFolderMB(), beaconID)); err != nil {
			fmt.Fprintf(c.App.Writer, "drand: beacon id [%s] - err reseting beacons database: %v\n", beaconID, err)
			os.Exit(1)
		}

		fmt.Printf("drand: beacon id [%s] - database reset\n", beaconID)
	}

	return nil
}

func askPort(c *cli.Context) string {
	for {
		fmt.Fprintf(c.App.Writer, "No valid port given. Please, choose a port number (or ENTER for default port 8080): ")

		reader := bufio.NewReader(c.App.Reader)
		input, err := reader.ReadString('\n')
		if err != nil {
			continue
		}

		portStr := strings.TrimSpace(input)
		if portStr == "" {
			fmt.Fprintln(c.App.Writer, "Default port selected")
			return defaultPort
		}

		port, err := strconv.Atoi(portStr)
		if err != nil || port < 1000 || port > 65536 {
			continue
		}

		return portStr
	}
}

func runMigration(c *cli.Context, l log.Logger) error {
	if err := checkArgs(c); err != nil {
		return err
	}

	config := contextToConfig(c, l)

	return migration.MigrateSBFolderStructure(config.ConfigFolder())
}

func checkMigration(c *cli.Context, l log.Logger) error {
	if err := checkArgs(c); err != nil {
		return err
	}

	config := contextToConfig(c, l)

	if isPresent := migration.CheckSBFolderStructure(config.ConfigFolder()); isPresent {
		return fmt.Errorf("single-beacon drand folder structure was not migrated, " +
			"please first do it with 'drand util migrate' command")
	}

	if fs.CreateSecureFolder(config.ConfigFolderMB()) == "" {
		return fmt.Errorf("something went wrong with the multi beacon folder. " +
			"Make sure that you have the appropriate rights")
	}

	return nil
}

func testWindows(c *cli.Context) error {
	// x509 not available on windows: must run without TLS
	if runtime.GOOS == "windows" && !c.Bool(insecureFlag.Name) {
		return errors.New("TLS is not available on Windows, please disable TLS")
	}
	return nil
}

func keygenCmd(c *cli.Context, l log.Logger) error {
	args := c.Args()
	if !args.Present() {
		return errors.New("missing drand address in argument. Abort")
	}

	if args.Len() > 1 {
		return fmt.Errorf("expecting only one argument, the address, but got:"+
			"\n\t%v\nAborting. Note that the flags need to go before the argument", args.Slice())
	}

	addr := args.First()
	var validID = regexp.MustCompile(`:\d+$`)
	if !validID.MatchString(addr) {
		fmt.Println("Invalid port:", addr)
		addr = addr + ":" + askPort(c)
	}

	sch, err := crypto.SchemeFromName(c.String(schemeFlag.Name))
	if err != nil {
		return err
	}

	var priv *key.Pair
	if c.Bool(insecureFlag.Name) {
		fmt.Println("Generating private / public key pair without TLS.")
		priv, err = key.NewKeyPair(addr, sch)
	} else {
		fmt.Println("Generating private / public key pair with TLS indication")
		priv, err = key.NewTLSKeyPair(addr, sch)
	}
	if err != nil {
		return err
	}

	config := contextToConfig(c, l)
	beaconID := getBeaconID(c)
	fileStore := key.NewFileStore(config.ConfigFolderMB(), beaconID)

	if _, err := fileStore.LoadKeyPair(sch); err == nil {
		keyDirectory := path.Join(config.ConfigFolderMB(), beaconID)
		fmt.Fprintf(c.App.Writer, "Keypair already present in `%s`.\nRemove them before generating new one\n", keyDirectory)
		return nil
	}
	if err := fileStore.SaveKeyPair(priv); err != nil {
		return fmt.Errorf("could not save key: %w", err)
	}

	fullpath := path.Join(config.ConfigFolderMB(), beaconID, key.FolderName)
	absPath, err := filepath.Abs(fullpath)

	if err != nil {
		return fmt.Errorf("err getting full path: %w", err)
	}
	fmt.Println("Generated keys at ", absPath)

	var buff bytes.Buffer
	if err := toml.NewEncoder(&buff).Encode(priv.Public.TOML()); err != nil {
		return err
	}

	buff.WriteString("\n")
	fmt.Println(buff.String())
	return nil
}

func groupOut(c *cli.Context, group *key.Group) error {
	if c.IsSet("out") {
		groupPath := c.String("out")
		if err := key.Save(groupPath, group, false); err != nil {
			return fmt.Errorf("drand: can't save group to specified file name: %w", err)
		}
	} else if c.Bool(hashOnly.Name) {
		fmt.Fprintf(c.App.Writer, "%x\n", group.Hash())
	} else {
		var buff bytes.Buffer
		if err := toml.NewEncoder(&buff).Encode(group.TOML()); err != nil {
			return fmt.Errorf("drand: can't encode group to TOML: %w", err)
		}
		buff.WriteString("\n")
		fmt.Fprintf(c.App.Writer, "The following group.toml file has been created\n")
		fmt.Fprint(c.App.Writer, buff.String())
		fmt.Fprintf(c.App.Writer, "\nHash of the group configuration: %x\n", group.Hash())
	}
	return nil
}

func checkIdentityAddress(lg log.Logger, conf *core.Config, addr string, tls bool, beaconID string) error {
	peer := net.CreatePeer(addr, tls)
	client := net.NewGrpcClientFromCertManager(lg, conf.Certs())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	metadata := &common2.Metadata{BeaconID: beaconID}
	identityResp, err := client.GetIdentity(ctx, peer, &drand.IdentityRequest{Metadata: metadata})
	if err != nil {
		return err
	}

	identity := &drand.Identity{
		Signature: identityResp.Signature,
		Tls:       identityResp.Tls,
		Address:   identityResp.Address,
		Key:       identityResp.Key,
	}
	sch, err := crypto.SchemeFromName(identityResp.SchemeName)
	if err != nil {
		lg.Errorw("received an invalid SchemeName in identity response", "received", identityResp.SchemeName)
		return err
	}
	id, err := key.IdentityFromProto(identity, sch)
	if err != nil {
		return err
	}
	if id.Address() != addr {
		return fmt.Errorf("mismatch of address: contact %s reply with %s", addr, id.Address())
	}
	return nil
}

func isVerbose(c *cli.Context) bool {
	return c.IsSet(verboseFlag.Name)
}

func logLevel(c *cli.Context) int {
	if isVerbose(c) {
		return log.DebugLevel
	}

	return log.ErrorLevel
}

func logJSON(c *cli.Context) bool {
	return c.Bool(jsonFlag.Name)
}

func toArray(flags ...cli.Flag) []cli.Flag {
	return flags
}
