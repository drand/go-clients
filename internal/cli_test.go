package drand

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClientTLS(t *testing.T) {
	addr := "https://api.drand.sh"

	chainInfoCmd := []string{"drand", "get", "chain-info", "--url", addr, "--insecure"}
	expectedInOutput := "868f005eb8e6e4ca0a47c8a77ceaa5309a47978a7c71bc5cce96366b5d7a569937c529eeda66c7293784a9402801af31"
	testCommand(t, chainInfoCmd, expectedInOutput)

	showHash := []string{"drand", "get", "public", "--url", addr, "--insecure", "123"}
	round1 := "0e4f538534f426203a4089154ff31527b9c25b37f9d6704b3ecba8d74678b4e3"
	testCommand(t, showHash, round1)
}

func testCommand(t *testing.T, args []string, exp string) {
	t.Helper()

	var buff bytes.Buffer
	t.Log("--------------")
	cli := CLI()
	cli.Writer = &buff
	require.NoError(t, cli.Run(args))
	if exp == "" {
		return
	}
	t.Logf("RUNNING: %v\n", args)
	require.Contains(t, strings.Trim(buff.String(), "\n"), exp)
}
