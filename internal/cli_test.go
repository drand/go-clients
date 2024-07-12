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
	expectedInOutput := "8990e7a9aaed2ffed73dbd7092123d6f289930540d7651336225dc172e51b2ce"
	testCommand(t, chainInfoCmd, expectedInOutput)

	showHash := []string{"drand", "get", "public", "--url", addr, "--insecure"}
	round1 := "101297f1ca7dc44ef6088d94ad5fb7ba03455dc33d53ddb412bbc4564ed986ec"
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
