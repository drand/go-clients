package lib

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	nhttp "net/http"
	"os"
	"path"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/google/uuid"
	bds "github.com/ipfs/go-ds-badger2"
	clock "github.com/jonboulle/clockwork"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/urfave/cli/v2"

	"github.com/drand/drand/v2/common/key"

	pubClient "github.com/drand/drand-cli/client"
	http2 "github.com/drand/drand-cli/client/http"
	gclient "github.com/drand/drand-cli/client/lp2p"
	"github.com/drand/drand-cli/internal/grpc"
	"github.com/drand/drand-cli/internal/lp2p"
	commonutils "github.com/drand/drand/v2/common"
	chainCommon "github.com/drand/drand/v2/common/chain"
	"github.com/drand/drand/v2/common/client"
	"github.com/drand/drand/v2/common/log"
)

var (
	// URLFlag is the CLI flag for root URL(s) for fetching randomness.
	URLFlag = &cli.StringSliceFlag{
		Name:  "url",
		Usage: "root URL(s) for fetching randomness",
	}
	// GRPCConnectFlag is the CLI flag for host:port to dial a gRPC randomness
	// provider.
	GRPCConnectFlag = &cli.StringFlag{
		Name:  "grpc-connect",
		Usage: "host:port to dial a gRPC randomness provider",
	}
	// HashFlag is the CLI flag for the hash (in hex) of the targeted chain.
	HashFlag = &cli.StringFlag{
		Name:    "hash",
		Usage:   "The hash (in hex) of the chain to follow. Deprecated and replaced by hash-list to support multiple chains",
		Aliases: []string{"chain-hash"},
		Hidden:  true,
	}
	// HashListFlag is the CLI flag for the hashes list (in hex) for the relay to follow.
	HashListFlag = &cli.StringSliceFlag{
		Name:  "hash-list",
		Usage: "Specify the list (in hex) of hashes the relay should follow",
	}
	// GroupConfFlag is the CLI flag for specifying the path to the drand group configuration (TOML encoded) or chain info (JSON encoded).
	GroupConfFlag = &cli.PathFlag{
		Name: "group-conf",
		Usage: "Path to a drand group configuration (TOML encoded) or chain info (JSON encoded)," +
			" can be used instead of `-hash` flag to verify the chain. Deprecated and replaced by group-conf-list to support multiple chains",
		Hidden: true,
	}
	// GroupConfListFlag is like GroupConfFlag but for a list values.
	GroupConfListFlag = &cli.StringSliceFlag{
		Name: "group-conf-list",
		Usage: "Paths to at least one drand group configuration (TOML encoded) or chain info (JSON encoded)," +
			fmt.Sprintf(" can be used instead of `-%s` flag to verify the chain.", HashListFlag.Name),
	}
	// InsecureFlag is the CLI flag to allow autodetection of the chain
	// information.
	InsecureFlag = &cli.BoolFlag{
		Name:  "insecure",
		Usage: "Allow autodetection of the chain information",
	}
	// RelayFlag is the CLI flag for relay peer multiaddr(s) to connect with.
	RelayFlag = &cli.StringSliceFlag{
		Name:  "relay",
		Usage: "relay peer multiaddr(s) to connect with",
	}
	// PortFlag is the CLI flag for local address for client to bind to, when
	// connecting to relays. (specified as a numeric port, or a host:port)
	PortFlag = &cli.StringFlag{
		Name:  "port",
		Usage: "Local (host:)port for constructed libp2p host to listen on",
	}

	// JSONFlag is the value of the CLI flag `json` enabling JSON output of the loggers
	JSONFlag = &cli.BoolFlag{
		Name:  "json",
		Usage: "Set the output as json format",
	}

	VerboseFlag = &cli.BoolFlag{
		Name:    "verbose",
		Usage:   "If set, verbosity is at the debug level",
		EnvVars: []string{"DRAND_VERBOSE"},
	}
)

// ClientFlags is a list of common flags for client creation
var ClientFlags = []cli.Flag{
	URLFlag,
	HashFlag,
	HashListFlag,
	GroupConfListFlag,
	GroupConfFlag,
	InsecureFlag,
	RelayFlag,
	JSONFlag,
	VerboseFlag,
}

// Create builds a client, and can be invoked from a cli action supplied
// with ClientFlags
//
//nolint:gocyclo
func Create(c *cli.Context, withInstrumentation bool, opts ...pubClient.Option) (client.Client, error) {
	ctx := c.Context
	clients := make([]client.Client, 0)
	var level int
	if c.Bool(VerboseFlag.Name) {
		level = log.DebugLevel
	} else {
		level = log.WarnLevel
	}
	l := log.New(nil, level, false)

	var info *chainCommon.Info
	var err error
	var hash []byte
	if groupPath := c.Path(GroupConfFlag.Name); groupPath != "" {
		info, err = chainInfoFromGroupTOML(groupPath)
		if err != nil {
			l.Infow("Got a group conf file that is not a toml file. Trying it as a ChainInfo json file.", "path", groupPath)
			info, err = chainInfoFromChainInfoJSON(groupPath)
			if info == nil || err != nil {
				return nil, fmt.Errorf("failed to decode group (%s) : %w", groupPath, err)
			}
		}
		opts = append(opts, pubClient.WithChainInfo(info))
	}

	if info != nil {
		hash = info.Hash()
	}

	grc, info, err := buildGrpcClient(c, info)
	if err != nil {
		return nil, err
	}
	if len(grc) > 0 {
		clients = append(clients, grc...)
	}

	if c.String(HashFlag.Name) != "" {
		hash, err = hex.DecodeString(c.String(HashFlag.Name))
		if err != nil {
			return nil, err
		}
		if info != nil && !bytes.Equal(hash, info.Hash()) {
			return nil, fmt.Errorf(
				"%w for beacon %s %v != %v",
				commonutils.ErrInvalidChainHash,
				info.ID,
				c.String(HashFlag.Name),
				hex.EncodeToString(info.Hash()),
			)
		}
		opts = append(opts, pubClient.WithChainHash(hash))
	}

	if c.Bool(InsecureFlag.Name) {
		opts = append(opts, pubClient.Insecurely())
	}

	gc, info, err := buildHTTPClients(c, l, hash, withInstrumentation)
	if err != nil {
		return nil, err
	}
	if len(gc) > 0 {
		clients = append(clients, gc...)
	}
	if info != nil && hash != nil && !bytes.Equal(hash, info.Hash()) {
		return nil, fmt.Errorf(
			"%w for beacon %s : expected %v != info %v",
			commonutils.ErrInvalidChainHash,
			info.ID,
			hex.EncodeToString(hash),
			hex.EncodeToString(info.Hash()),
		)
	}

	gopt, err := buildGossipClient(c, l)
	if err != nil {
		return nil, err
	}
	opts = append(opts, gopt...)

	return pubClient.Wrap(ctx, l, clients, opts...)
}

func buildGrpcClient(c *cli.Context, info *chainCommon.Info) ([]client.Client, *chainCommon.Info, error) {
	if !c.IsSet(GRPCConnectFlag.Name) {
		return nil, info, nil
	}

	var hash []byte
	if c.IsSet(HashFlag.Name) {
		var err error

		hash, err = hex.DecodeString(c.String(HashFlag.Name))
		if err != nil {
			return nil, nil, err
		}
	}

	if info != nil && len(hash) == 0 {
		hash = info.Hash()
	}

	gc, err := grpc.New(c.String(GRPCConnectFlag.Name), c.Bool(InsecureFlag.Name), hash)
	if err != nil {
		return nil, nil, err
	}

	if info == nil {
		info, err = gc.Info(c.Context)
		if err != nil {
			return nil, nil, err
		}
	}

	return []client.Client{gc}, info, nil
}

func buildHTTPClients(c *cli.Context, l log.Logger, hash []byte, withInstrumentation bool) ([]client.Client, *chainCommon.Info, error) {
	ctx := c.Context
	clients := make([]client.Client, 0)
	var err error
	var skipped []string
	var hc client.Client
	var info *chainCommon.Info

	l.Infow("Building HTTP clients", "hash", len(hash), "urls", c.StringSlice(URLFlag.Name))

	for _, url := range c.StringSlice(URLFlag.Name) {
		hc, err = http2.New(ctx, l, url, hash, nhttp.DefaultTransport)
		if err != nil {
			l.Warnw("", "client", "failed to load URL", "url", url, "err", err)
			skipped = append(skipped, url)
			continue
		}
		info, err = hc.Info(ctx)
		if err != nil {
			l.Warnw("", "client", "failed to load Info from URL", "url", url, "err", err)
			continue
		}

		clients = append(clients, hc)
	}

	if len(skipped) == len(c.StringSlice(URLFlag.Name)) {
		return nil, nil, errors.New("all URLs failed to be used for creating a http client")
	}

	if info != nil {
		if hash != nil && !bytes.Equal(hash, info.Hash()) {
			l.Warnw("mismatch between retrieved chain info hash and provided hash", "chainInfo", info.Hash(), "provided", hash)
			return nil, nil, errors.New("mismatch between retrieved chain info and provided hash")
		}

		for _, url := range skipped {
			hc, err = http2.NewWithInfo(l, url, info, nhttp.DefaultTransport)
			if err != nil {
				l.Warnw("", "client", "failed to load URL", "url", url, "err", err)
				continue
			}
			clients = append(clients, hc)
		}
	}

	if withInstrumentation {
		http2.MeasureHeartbeats(c.Context, clients)
	}

	return clients, info, nil
}

func buildGossipClient(c *cli.Context, l log.Logger) ([]pubClient.Option, error) {
	if c.IsSet(RelayFlag.Name) {
		addrs := c.StringSlice(RelayFlag.Name)
		if len(addrs) > 0 {
			relayPeers, err := lp2p.ParseMultiaddrSlice(addrs)
			if err != nil {
				return nil, err
			}
			listen := ""
			if c.IsSet(PortFlag.Name) {
				listen = c.String(PortFlag.Name)
			}
			ps, err := buildClientHost(l, listen, relayPeers)
			if err != nil {
				return nil, err
			}
			return []pubClient.Option{gclient.WithPubsub(l, ps, clock.NewRealClock(), gclient.DefaultBufferSize)}, nil
		}
	}
	return []pubClient.Option{}, nil
}

func buildClientHost(l log.Logger, clientListenAddr string, relayMultiaddr []ma.Multiaddr) (*pubsub.PubSub, error) {
	clientID := uuid.New().String()
	ds, err := bds.NewDatastore(path.Join(os.TempDir(), "drand-"+clientID+"-datastore"), nil)
	if err != nil {
		return nil, err
	}
	priv, err := lp2p.LoadOrCreatePrivKey(path.Join(os.TempDir(), "drand-"+clientID+"-id"), l)
	if err != nil {
		return nil, err
	}

	listen := ""
	if clientListenAddr != "" {
		bindHost := "0.0.0.0"
		if strings.Contains(clientListenAddr, ":") {
			host, port, err := net.SplitHostPort(clientListenAddr)
			if err != nil {
				return nil, err
			}
			bindHost = host
			clientListenAddr = port
		}
		listen = fmt.Sprintf("/ip4/%s/tcp/%s", bindHost, clientListenAddr)
	}

	_, ps, err := lp2p.ConstructHost(
		ds,
		priv,
		listen,
		relayMultiaddr,
		l,
	)
	if err != nil {
		return nil, err
	}
	return ps, nil
}

// chainInfoFromGroupTOML reads a drand group TOML file and returns the chain info.
func chainInfoFromGroupTOML(filePath string) (*chainCommon.Info, error) {
	gt := &key.GroupTOML{}
	_, err := toml.DecodeFile(filePath, gt)
	if err != nil {
		return nil, err
	}
	g := &key.Group{}
	err = g.FromTOML(gt)
	if err != nil {
		return nil, err
	}
	return chainCommon.NewChainInfo(g), nil
}

func chainInfoFromChainInfoJSON(filePath string) (*chainCommon.Info, error) {
	b, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	return chainCommon.InfoFromJSON(bytes.NewBuffer(b))
}
