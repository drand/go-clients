package http

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	nhttp "net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/drand/drand/v2/crypto"
	"github.com/drand/go-clients/client"
	"github.com/drand/go-clients/drand"

	json "github.com/nikkolasg/hexjson"

	"github.com/drand/drand/v2/common"
	chain2 "github.com/drand/drand/v2/common/chain"
	"github.com/drand/drand/v2/common/log"
)

var _ drand.Client = &httpClient{}
var _ drand.LoggingClient = &httpClient{}

var errClientClosed = fmt.Errorf("client closed")

const defaultClientExec = "unknown"
const defaultHTTTPTimeout = 60 * time.Second

const httpWaitMaxCounter = 20
const httpWaitInterval = 2 * time.Second
const maxTimeoutHTTPRequest = 5 * time.Second

// NewSimpleClient creates a client using the default logger, default transport and a background context
// to instantiate a new Client for that remote host for that specific chainhash.
func NewSimpleClient(host, chainhash string) (*httpClient, error) {
	chb, err := hex.DecodeString(chainhash)
	if err != nil {
		return nil, fmt.Errorf("unable to create basic HTTP client for url %q and chainhash %q: %w", host, chainhash, err)
	}
	return New(context.Background(), nil, host, chb, nil)
}

// New creates a new client pointing to an HTTP endpoint
func New(ctx context.Context, l log.Logger, url string, chainHash []byte, transport nhttp.RoundTripper) (*httpClient, error) {
	if l == nil {
		l = log.DefaultLogger()
	}
	if transport == nil {
		transport = nhttp.DefaultTransport
	}
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	pn, err := os.Executable()
	if err != nil {
		pn = defaultClientExec
	}
	agent := fmt.Sprintf("go-client-%s/2.0", path.Base(pn))
	c := &httpClient{
		root:   url,
		client: createClient(transport),
		l:      l,
		Agent:  agent,
		done:   make(chan struct{}),
	}

	chainInfo, err := c.FetchChainInfo(ctx, chainHash)
	if err != nil {
		return nil, fmt.Errorf("FetchChainInfo err: %w", err)
	}
	c.chainInfo = chainInfo

	return c, nil
}

// NewWithInfo constructs an http client when the group parameters are already known.
func NewWithInfo(l log.Logger, url string, info *chain2.Info, transport nhttp.RoundTripper) (*httpClient, error) {
	if l == nil {
		l = log.DefaultLogger()
	}
	if transport == nil {
		transport = nhttp.DefaultTransport
	}
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}

	pn, err := os.Executable()
	if err != nil {
		pn = defaultClientExec
	}
	agent := fmt.Sprintf("drand-client-%s/1.0", path.Base(pn))
	c := &httpClient{
		root:      url,
		chainInfo: info,
		client:    createClient(transport),
		l:         l,
		Agent:     agent,
		done:      make(chan struct{}),
	}
	return c, nil
}

// ForURLs provides a shortcut for creating a set of HTTP clients for a set of URLs.
func ForURLs(ctx context.Context, l log.Logger, urls []string, chainHash []byte) []drand.Client {
	clients := make([]drand.Client, 0)
	var info *chain2.Info
	var skipped []string
	for _, u := range urls {
		if info == nil {
			if c, err := New(ctx, l, u, chainHash, nil); err == nil {
				// Note: this wrapper assumes the current behavior that if `New` succeeds,
				// Info will have been fetched.
				info, _ = c.Info(ctx)
				clients = append(clients, c)
			} else {
				skipped = append(skipped, u)
			}
		} else {
			if c, err := NewWithInfo(l, u, info, nil); err == nil {
				clients = append(clients, c)
			}
		}
	}
	if info != nil {
		for _, u := range skipped {
			if c, err := NewWithInfo(l, u, info, nil); err == nil {
				clients = append(clients, c)
			}
		}
	}
	return clients
}

func Ping(ctx context.Context, root string) error {
	url := fmt.Sprintf("%s/health", root)

	ctx, cancel := context.WithTimeout(ctx, maxTimeoutHTTPRequest)
	defer cancel()

	req, err := nhttp.NewRequestWithContext(ctx, nhttp.MethodGet, url, nhttp.NoBody)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	response, err := nhttp.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	defer response.Body.Close()

	return nil
}

// createClient creates an HTTP client around a transport, allows to easily instrument it later
func createClient(transport nhttp.RoundTripper) *nhttp.Client {
	hc := nhttp.Client{}
	hc.Timeout = defaultHTTTPTimeout
	hc.Jar = nhttp.DefaultClient.Jar
	hc.CheckRedirect = nhttp.DefaultClient.CheckRedirect
	hc.Transport = transport

	return &hc
}

func IsServerReady(ctx context.Context, addr string) error {
	counter := 0
	for {
		// Ping is wrapping its context with a Timeout on maxTimeoutHTTPRequest anyway.
		err := Ping(ctx, "http://"+addr)
		if err == nil {
			return nil
		}

		counter++
		if counter == httpWaitMaxCounter {
			return fmt.Errorf("timeout waiting http server to be ready")
		}

		time.Sleep(httpWaitInterval)
	}
}

// httpClient implements Client through http requests to a Drand relay.
type httpClient struct {
	root      string
	client    *nhttp.Client
	Agent     string
	chainInfo *chain2.Info
	l         log.Logger
	done      chan struct{}
}

// SetLog configures the client log output
func (h *httpClient) SetLog(l log.Logger) {
	h.l = l
}

// SetUserAgent sets the user agent used by the client
func (h *httpClient) SetUserAgent(ua string) {
	h.Agent = ua
}

// String returns the name of this client.
func (h *httpClient) String() string {
	return fmt.Sprintf("HTTP(%q)", h.root)
}

// MarshalText implements encoding.TextMarshaller interface
func (h *httpClient) MarshalText() ([]byte, error) {
	return json.Marshal(h.String())
}

type httpInfoResponse struct {
	chainInfo *chain2.Info
	err       error
}

// FetchChainInfo attempts to initialize an httpClient when
// it does not know the full group parameters for a drand group. The chain hash
// is the hash of the chain info.
func (h *httpClient) FetchChainInfo(ctx context.Context, chainHash []byte) (*chain2.Info, error) {
	if h.chainInfo != nil {
		return h.chainInfo, nil
	}

	resC := make(chan httpInfoResponse, 1)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		var url string
		if len(chainHash) > 0 {
			url = fmt.Sprintf("%s%x/info", h.root, chainHash)
		} else {
			url = fmt.Sprintf("%sinfo", h.root)
		}

		req, err := nhttp.NewRequestWithContext(ctx, nhttp.MethodGet, url, nhttp.NoBody)
		if err != nil {
			resC <- httpInfoResponse{nil, fmt.Errorf("creating request: %w", err)}
			return
		}
		req.Header.Set("User-Agent", h.Agent)

		infoBody, err := h.client.Do(req)
		if err != nil {
			resC <- httpInfoResponse{nil, fmt.Errorf("doing request: %w", err)}
			return
		}
		defer infoBody.Body.Close()

		chainInfo, err := chain2.InfoFromJSON(infoBody.Body)
		if err != nil {
			resC <- httpInfoResponse{nil, fmt.Errorf("decoding response [InfoFromJSON]: %w", err)}
			return
		}

		if chainInfo.PublicKey == nil {
			resC <- httpInfoResponse{nil, fmt.Errorf("group does not have a valid key for validation")}
			return
		}

		if len(chainHash) == 0 {
			h.l.Warnw("", "http_client", "instantiated without trustroot", "chainHash", hex.EncodeToString(chainInfo.Hash()))
			if !common.IsDefaultBeaconID(chainInfo.ID) {
				err := fmt.Errorf("%s does not advertise the default drand for the default chainHash (got %x)", h.root, chainInfo.Hash())
				resC <- httpInfoResponse{nil, err}
				return
			}
		} else if !bytes.Equal(chainInfo.Hash(), chainHash) {
			err := fmt.Errorf("%s does not advertise the expected drand group (%x vs %x)", h.root, chainInfo.Hash(), chainHash)
			resC <- httpInfoResponse{nil, err}
			return
		}

		resC <- httpInfoResponse{chainInfo, nil}
	}()

	select {
	case res := <-resC:
		if res.err != nil {
			return nil, res.err
		}
		return res.chainInfo, nil
	case <-h.done:
		return nil, errClientClosed
	}
}

type httpGetResponse struct {
	result drand.Result
	err    error
}

// Get returns the randomness at `round` or an error.
func (h *httpClient) Get(ctx context.Context, round uint64) (drand.Result, error) {
	var url string
	if round == 0 {
		url = fmt.Sprintf("%s%x/public/latest", h.root, h.chainInfo.Hash())
	} else {
		url = fmt.Sprintf("%s%x/public/%d", h.root, h.chainInfo.Hash(), round)
	}

	resC := make(chan httpGetResponse, 1)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		req, err := nhttp.NewRequestWithContext(ctx, nhttp.MethodGet, url, nhttp.NoBody)
		if err != nil {
			resC <- httpGetResponse{nil, fmt.Errorf("creating request: %w", err)}
			return
		}
		req.Header.Set("User-Agent", h.Agent)

		randResponse, err := h.client.Do(req)
		if err != nil {
			resC <- httpGetResponse{nil, fmt.Errorf("error doing GET request to %q: %w", url, err)}
			return
		}
		if randResponse.StatusCode != nhttp.StatusOK {
			resC <- httpGetResponse{nil, fmt.Errorf("got invalid status %d doing GET request to %q", randResponse.StatusCode, url)}
			return
		}
		defer randResponse.Body.Close()

		randResp := client.RandomData{}
		if err := json.NewDecoder(randResponse.Body).Decode(&randResp); err != nil {
			resC <- httpGetResponse{nil, fmt.Errorf("decoding response: %w", err)}
			return
		}

		if len(randResp.Sig) == 0 {
			resC <- httpGetResponse{nil, fmt.Errorf("insufficient response - signature is not present")}
			return
		}

		randResp.Random = crypto.RandomnessFromSignature(randResp.GetSignature())

		resC <- httpGetResponse{&randResp, nil}
	}()

	select {
	case res := <-resC:
		if res.err != nil {
			return nil, res.err
		}
		return res.result, nil
	case <-h.done:
		return nil, errClientClosed
	}
}

// Watch returns new randomness as it becomes available.
func (h *httpClient) Watch(ctx context.Context) <-chan drand.Result {
	out := make(chan drand.Result)
	go func() {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		defer close(out)

		in := client.PollingWatcher(ctx, h, h.chainInfo, h.l)
		for {
			select {
			case res, ok := <-in:
				if !ok {
					return
				}
				out <- res
			case <-h.done:
				return
			}
		}
	}()
	return out
}

// Info returns information about the chain.
func (h *httpClient) Info(_ context.Context) (*chain2.Info, error) {
	return h.chainInfo, nil
}

// RoundAt will return the most recent round of randomness that will be available
// at time for the current client.
func (h *httpClient) RoundAt(t time.Time) uint64 {
	return common.CurrentRound(t.Unix(), h.chainInfo.Period, h.chainInfo.GenesisTime)
}

func (h *httpClient) Close() error {
	close(h.done)
	h.client.CloseIdleConnections()
	return nil
}
