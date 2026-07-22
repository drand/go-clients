package lib

import (
	"bytes"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	clock "github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"

	"github.com/drand/go-clients/drand"

	"github.com/drand/drand/v2/common/log"
	"github.com/drand/drand/v2/crypto"
	"github.com/drand/go-clients/client"
	httpmock "github.com/drand/go-clients/client/test/http/mock"
)

var (
	opts []client.Option
)

const (
	fakeGossipRelayAddr = "/ip4/8.8.8.8/tcp/9/p2p/QmSoLju6m7xTh3DuokvT3886QRYqxAzb1kShaanJgW36yx"
	fakeChainHash       = "6093f9e4320c285ac4aab50ba821cd5678ec7c5015d3d9d11ef89e2a99741e83"
)

func mockAction(c *cli.Context) error {
	_, err := Create(c, false, opts...)
	return err
}

func run(l log.Logger, args []string) error {
	app := cli.NewApp()
	app.Name = "mock-client"
	app.Flags = ClientFlags
	app.Action = func(c *cli.Context) error {
		c.Context = log.ToContext(c.Context, l)
		return mockAction(c)
	}

	return app.Run(args)
}

func TestClientLib(t *testing.T) {
	opts = []client.Option{}
	lg := log.New(nil, log.DebugLevel, true)
	err := run(lg, []string{"mock-client"})
	if err == nil {
		t.Fatal("need to specify a connection method.", err)
	}

	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	clk := clock.NewFakeClockAt(time.Now())
	addr, info, cancel, _ := httpmock.NewMockHTTPPublicServer(t, false, sch, clk)
	defer cancel()

	t.Log("Started mockserver at", addr)

	args := []string{"mock-client", "--url", "http://" + addr, "--insecure"}
	err = run(lg, args)
	if err != nil {
		t.Fatal("HTTP should work. err:", err)
	}

	args = []string{"mock-client", "--url", "https://" + addr}
	err = run(lg, args)
	if err == nil {
		t.Fatal("http-relay needs insecure or hash", err)
	}

	args = []string{"mock-client", "--url", "http://" + addr, "--hash", hex.EncodeToString(info.Hash())}
	err = run(lg, args)
	if err != nil {
		t.Fatal("http-relay should construct", err)
	}

	args = []string{"mock-client", "--relay", fakeGossipRelayAddr}
	err = run(lg, args)
	if err == nil {
		t.Fatal("relays need URL to get chain info and hash", err)
	}

	args = []string{"mock-client", "--relay", fakeGossipRelayAddr, "--hash", hex.EncodeToString(info.Hash())}
	err = run(lg, args)
	if err == nil {
		t.Fatal("relays need URL to get chain info and hash", err)
	}

	args = []string{"mock-client", "--url", "http://" + addr, "--relay", fakeGossipRelayAddr, "--hash", hex.EncodeToString(info.Hash())}
	err = run(lg, args)
	if err != nil {
		t.Fatal("unable to get relay to work", err)
	}
}

func TestClientLibGroupConfTOML(t *testing.T) {
	lg := log.New(nil, log.DebugLevel, true)
	err := run(lg, []string{"mock-client", "--relay", fakeGossipRelayAddr, "--group-conf", groupTOMLPath()})
	if err != nil {
		t.Fatal(err)
	}
}

func TestClientLibGroupConfJSON(t *testing.T) {
	lg := log.New(nil, log.DebugLevel, true)
	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	clk := clock.NewFakeClockAt(time.Now())

	addr, info, cancel, _ := httpmock.NewMockHTTPPublicServer(t, false, sch, clk)
	defer cancel()

	var b bytes.Buffer
	require.NoError(t, info.ToJSON(&b, nil))

	infoPath := filepath.Join(t.TempDir(), "info.json")

	err = os.WriteFile(infoPath, b.Bytes(), 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = run(lg, []string{"mock-client", "--url", "http://" + addr, "--group-conf", infoPath})
	if err != nil {
		t.Fatal(err)
	}
}

func TestClientLibChainHashOverrideError(t *testing.T) {
	lg := log.New(nil, log.DebugLevel, true)
	err := run(lg, []string{
		"mock-client",
		"--relay",
		fakeGossipRelayAddr,
		"--group-conf",
		groupTOMLPath(),
		"--hash",
		fakeChainHash,
	})
	if !errors.Is(err, drand.ErrInvalidChainHash) {
		t.Log(fakeChainHash)
		t.Fatal("expected error from mismatched chain hashes. Got: ", err)
	}
}

func groupTOMLPath() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "internal", "testdata", "default.toml")
}

// TestClientLibHashListFlag covers --hash-list being honoured as a root of
// trust. It is registered on the client commands, but Create used to read only
// --hash, so passing it left the client with no root of trust at all.
func TestClientLibHashListFlag(t *testing.T) {
	opts = []client.Option{}
	lg := log.New(nil, log.DebugLevel, true)

	sch, err := crypto.GetSchemeFromEnv()
	require.NoError(t, err)
	clk := clock.NewFakeClockAt(time.Now())
	addr, info, cancel, _ := httpmock.NewMockHTTPPublicServer(t, false, sch, clk)
	defer cancel()

	// the correct hash is accepted and serves as the root of trust
	args := []string{"mock-client", "--url", "http://" + addr, "--hash-list", hex.EncodeToString(info.Hash())}
	require.NoError(t, run(lg, args), "--hash-list should pin the chain")

	// a mismatching hash is rejected rather than silently ignored
	args = []string{"mock-client", "--url", "http://" + addr, "--hash-list", hex.EncodeToString(make([]byte, 32))}
	require.Error(t, run(lg, args), "--hash-list should reject a chain hash mismatch")

	// a client follows a single chain, so several hashes are ambiguous
	args = []string{
		"mock-client", "--url", "http://" + addr,
		"--hash-list", hex.EncodeToString(info.Hash()),
		"--hash-list", hex.EncodeToString(make([]byte, 32)),
	}
	require.Error(t, run(lg, args), "several chain hashes should be refused")
}

// TestClientLibGroupConfListFlag covers the same silent-drop for
// --group-conf-list on the client path.
func TestClientLibGroupConfListFlag(t *testing.T) {
	opts = []client.Option{}
	lg := log.New(nil, log.DebugLevel, true)

	args := []string{"mock-client", "--relay", fakeGossipRelayAddr, "--group-conf-list", groupTOMLPath()}
	require.NoError(t, run(lg, args), "--group-conf-list should be honoured")
}

// TestChainInfoFromGroupTOMLWithoutPublicKey ensures a group file that has no
// distributed public key -- a group proposal that never went through a DKG --
// is reported as a decode error. NewChainInfo dereferences the key
// unconditionally, so this used to panic with a nil pointer dereference.
func TestChainInfoFromGroupTOMLWithoutPublicKey(t *testing.T) {
	src, err := os.ReadFile(groupTOMLPath())
	require.NoError(t, err)

	// drop the [PublicKey] section and everything after it
	idx := bytes.Index(src, []byte("[PublicKey]"))
	require.Positive(t, idx, "test fixture should contain a [PublicKey] section")

	path := filepath.Join(t.TempDir(), "nokey.toml")
	require.NoError(t, os.WriteFile(path, src[:idx], 0o600))

	require.NotPanics(t, func() {
		_, err := chainInfoFromGroupTOML(path)
		require.Error(t, err, "a group without a public key should not decode")
	})
}
