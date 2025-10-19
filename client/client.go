package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/drand/go-clients/drand"

	"github.com/drand/drand/v2/common/chain"
	"github.com/drand/drand/v2/common/log"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/go-clients/internal/metrics"
)

const ClientStartupTimeout = time.Second * 5

// New creates a watcher, verifying, optimizing client with the specified options.
// It expects to be provided at least 1 valid client, or more using From().
// If not specified, a default context with a timeout of ClientStartupTimeout
// will be used when fetching chain information during client setup.
func New(options ...Option) (drand.Client, error) {
	cfg := clientConfig{
		cacheSize: 32,
	}

	for _, opt := range options {
		if err := opt(&cfg); err != nil {
			return nil, err
		}
	}
	if cfg.log == nil {
		cfg.log = log.DefaultLogger()
	}
	if cfg.setupCtx == nil {
		ctx, cancel := context.WithTimeout(context.Background(), ClientStartupTimeout)
		cfg.setupCtx = ctx
		defer cancel()
	}
	return makeClient(&cfg)
}

// Wrap provides a single entrypoint for wrapping a concrete client
// implementation with configured aggregation, caching, and retry logic.
// It calls New and has the same expectations.
func Wrap(clients []drand.Client, options ...Option) (drand.Client, error) {
	return New(append(options, From(clients...))...)
}

func trySetLog(c any, l log.Logger) {
	if lc, ok := c.(drand.LoggingClient); ok {
		lc.SetLog(l)
	}
}

// makeClient creates a watching verifying optimizing client from a configuration.
func makeClient(cfg *clientConfig) (drand.Client, error) {
	l := cfg.log
	if !cfg.insecure && cfg.chainHash == nil && cfg.chainInfo == nil {
		l.Errorw("no root of trust specified")
		return nil, errors.New("no root of trust specified")
	}
	if len(cfg.clients) == 0 && cfg.watcher == nil {
		l.Errorw("no points of contact specified")
		return nil, errors.New("no points of contact specified")
	}

	var err error

	// provision cache
	cache, err := makeCache(cfg.cacheSize)
	if err != nil {
		return nil, err
	}

	// try to populate chain info
	if err := cfg.tryPopulateInfo(cfg.setupCtx, cfg.clients...); err != nil {
		return nil, err
	}

	// provision watcher client
	var wc drand.Client
	if cfg.watcher != nil {
		wc, err = makeWatcherClient(cfg, cache)
		if err != nil {
			return nil, err
		}
		cfg.clients = append(cfg.clients, wc)
	}

	// bind prometheus metrics if a registerer was provided
	if cfg.prometheus != nil {
		// ignore registration errors; caller may re-use registries across clients
		_ = metrics.RegisterClientMetrics(cfg.prometheus)
	}

	for _, c := range cfg.clients {
		trySetLog(c, cfg.log)
	}

	var c drand.Client

	verifiers := make([]drand.Client, 0, len(cfg.clients))
	for _, source := range cfg.clients {
		sch, err := crypto.GetSchemeByID(cfg.chainInfo.Scheme)
		if err != nil {
			return nil, fmt.Errorf("invalid scheme name in makeClient: %w", err)
		}

		nv := newVerifyingClient(source, cfg.previousResult, cfg.fullVerify, sch)
		verifiers = append(verifiers, nv)
		if source == wc {
			wc = nv
		}
	}

	c, err = makeOptimizingClient(l, cfg, verifiers, wc, cache)
	if err != nil {
		return nil, err
	}

	wa := newWatchAggregator(l, c, wc, cfg.autoWatch, cfg.autoWatchRetry)
	c = wa
	trySetLog(c, cfg.log)

	wa.Start()

	return c, nil
}

//nolint:lll // This function has nicely named parameters, so it's long.
func makeOptimizingClient(l log.Logger, cfg *clientConfig, verifiers []drand.Client, watcher drand.Client, cache Cache) (drand.Client, error) {
	oc, err := newOptimizingClient(l, verifiers, 0, 0, 0, 0)
	if err != nil {
		return nil, err
	}
	if watcher != nil {
		oc.MarkPassive(watcher)
	}
	c := drand.Client(oc)
	trySetLog(c, cfg.log)

	if cfg.cacheSize > 0 {
		c, err = NewCachingClient(l, c, cache)
		if err != nil {
			return nil, err
		}
		trySetLog(c, cfg.log)
	}
	for _, v := range verifiers {
		trySetLog(v, cfg.log)
		v.(*verifyingClient).indirectClient = c
	}

	oc.Start()
	return c, nil
}

func makeWatcherClient(cfg *clientConfig, cache Cache) (drand.Client, error) {
	if cfg.chainInfo == nil {
		return nil, fmt.Errorf("chain info cannot be nil")
	}

	w, err := cfg.watcher(cfg.log, cfg.chainInfo, cache)
	if err != nil {
		return nil, err
	}
	ec := EmptyClientWithInfo(cfg.chainInfo)
	return &watcherClient{ec, w}, nil
}

type clientConfig struct {
	// clients is the set of options for fetching randomness
	clients []drand.Client
	// watcher is a constructor function for generating a new partial client of randomness
	watcher WatcherCtor
	// from `chainInfo.Hash()` - serves as a root of trust for a given
	// randomness chain.
	chainHash []byte
	// Full chain information - serves as a root of trust.
	chainInfo *chain.Info
	// A previously fetched result serving as a verification checkpoint if one exists.
	previousResult drand.Result
	// chain signature verification back to the 1st round, or to a know result to ensure
	// determinism in the event of a compromised chain.
	fullVerify bool
	// insecure indicates the root of trust does not need to be present.
	insecure bool
	// autoWatch causes the client to start watching immediately in the background so that new randomness
	// is proactively fetched and added to the cache.
	autoWatch bool
	// cache size - how large of a cache to keep locally.
	cacheSize int
	// customized client log.
	log log.Logger

	// only used during setup to try and fetch chain info if chain info is nil
	setupCtx context.Context

	// autoWatchRetry specifies the time after which the watch channel
	// created by the autoWatch is re-opened when no context error occurred.
	autoWatchRetry time.Duration
	// prometheus is an interface to a Prometheus system
	prometheus prometheus.Registerer
}

func (c *clientConfig) tryPopulateInfo(ctx context.Context, clients ...drand.Client) (err error) {
	if c.chainInfo == nil {
		var cerr error
		for _, cli := range clients {
			c.chainInfo, cerr = cli.Info(ctx)
			if cerr == nil {
				return
			}
			// we accumulate errors to try all clients even if the first one fails
			err = errors.Join(err, cerr, ctx.Err())
		}
	}
	return
}

// Option is an option configuring a client.
type Option func(cfg *clientConfig) error

// From constructs the client from a set of clients providing randomness
func From(c ...drand.Client) Option {
	return func(cfg *clientConfig) error {
		cfg.clients = c
		return nil
	}
}

// Insecurely indicates the client should be allowed to provide randomness
// when the root of trust is not fully provided in a validate-able way.
func Insecurely() Option {
	return func(cfg *clientConfig) error {
		cfg.insecure = true
		return nil
	}
}

// WithCacheSize specifies how large of a cache of randomness values should be
// kept locally. Default 32
func WithCacheSize(size int) Option {
	return func(cfg *clientConfig) error {
		cfg.cacheSize = size
		return nil
	}
}

// WithChainHash configures the client to root trust with a given randomness
// chain hash, the chain parameters will be fetched from an HTTP endpoint.
func WithChainHash(chainHash []byte) Option {
	return func(cfg *clientConfig) error {
		if cfg.chainInfo != nil && !bytes.Equal(cfg.chainInfo.Hash(), chainHash) {
			return errors.New("refusing to override group with non-matching hash")
		}
		cfg.chainHash = chainHash
		return nil
	}
}

// WithChainInfo configures the client to root trust in the given randomness
// chain information, this prevents the setup of the client from attempting to
// fetch the chain info through the clients from the remotes.
func WithChainInfo(chainInfo *chain.Info) Option {
	return func(cfg *clientConfig) error {
		if cfg.chainHash != nil && !bytes.Equal(cfg.chainHash, chainInfo.Hash()) {
			return errors.New("refusing to override hash with non-matching group")
		}
		cfg.chainInfo = chainInfo
		return nil
	}
}

// WithLogger overrides the logging options for the client,
// allowing specification of additional tags, or redirection / configuration
// of logging level and output. If it is not used to set a specific logger,
// the client's logger will be used. This only works for clients that satisfy
// the drand.LoggingClient interface.
func WithLogger(l log.Logger) Option {
	return func(cfg *clientConfig) error {
		cfg.log = l
		return nil
	}
}

// WithSetupCtx allows you to provide a custom setup context that will be used
// if WithChainInfo isn't used and the client setup has to try and fetch the
// ChainInfo from the remotes.
func WithSetupCtx(ctx context.Context) Option {
	return func(cfg *clientConfig) error {
		cfg.setupCtx = ctx
		return nil
	}
}

// WithTrustedResult provides a checkpoint of randomness verified at a given round.
// Used in combination with `VerifyFullChain`, this allows for catching up only on
// previously not-yet-verified results. Note that in general this is not something you need.
func WithTrustedResult(result drand.Result) Option {
	return func(cfg *clientConfig) error {
		if cfg.previousResult != nil && cfg.previousResult.GetRound() > result.GetRound() {
			return errors.New("refusing to override verified result with an earlier result")
		}
		cfg.previousResult = result
		return nil
	}
}

// WithFullChainVerification validates random beacons using the chained schemes are
// not just as being generated correctly from the group signature,
// but ensures that the full chain is deterministic by making sure
// each round is derived correctly from the previous one. In cases of compromise where
// a single party learns sufficient shares to derive the full key, malicious randomness
// could otherwise be generated that is signed, but not properly derived from previous rounds
// according to protocol. Note that in general this is not something you need.
func WithFullChainVerification() Option {
	return func(cfg *clientConfig) error {
		cfg.fullVerify = true
		return nil
	}
}

// Watcher supplies the `Watch` portion of the drand client interface.
type Watcher interface {
	Watch(ctx context.Context) <-chan drand.Result
}

// WatcherCtor creates a Watcher once chain info is known.
type WatcherCtor func(l log.Logger, chainInfo *chain.Info, cache Cache) (Watcher, error)

// WithWatcher specifies a channel that can provide notifications of new
// randomness bootstrappeed from the chain info.
func WithWatcher(wc WatcherCtor) Option {
	return func(cfg *clientConfig) error {
		cfg.watcher = wc
		return nil
	}
}

// WithAutoWatch causes the client to automatically attempt to get
// randomness for rounds, so that it will hopefully already be cached
// when `Get` is called.
func WithAutoWatch() Option {
	return func(cfg *clientConfig) error {
		cfg.autoWatch = true
		return nil
	}
}

// WithAutoWatchRetry specifies the time after which the watch channel
// created by the autoWatch is re-opened when no context error occurred.
// Set to a negative value to disable retrying auto watch.
func WithAutoWatchRetry(interval time.Duration) Option {
	return func(cfg *clientConfig) error {
		cfg.autoWatchRetry = interval
		return nil
	}
}

// WithPrometheus specifies a registry into which to report metrics
func WithPrometheus(r prometheus.Registerer) Option {
	return func(cfg *clientConfig) error {
		cfg.prometheus = r
		return nil
	}
}
