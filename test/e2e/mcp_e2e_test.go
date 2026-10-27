//go:build e2e

package e2e_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestMCPServerSpeaksOnlyProtocolOnStdout starts the real binary. A stray line
// on stdout would break the JSON-RPC framing and fail the handshake.
func TestMCPServerSpeaksOnlyProtocolOnStdout(t *testing.T) {
	bin := buildBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.Command(bin, "mcp")
	cmd.Env = append(os.Environ(),
		"SBOX_DEFAULT_PROVIDER=sprites",
		"SBOX_SPRITES_TOKEN=e2e-not-a-real-token",
	)
	var stderr strings.Builder
	cmd.Stderr = &stderr

	client := sdk.NewClient(&sdk.Implementation{Name: "e2e", Version: "v0"}, nil)
	session, err := client.Connect(ctx, &sdk.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("connect: %v\nstderr: %s", err, stderr.String())
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	for _, want := range []string{"list_providers", "create_sandbox", "exec", "destroy"} {
		if !slices.Contains(names, want) {
			t.Errorf("tool %q missing from %v", want, names)
		}
	}

	res, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "list_providers"})
	if err != nil {
		t.Fatalf("list_providers: %v", err)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out struct {
		Providers []struct {
			Name      string `json:"name"`
			Available bool   `json:"available"`
			Default   bool   `json:"default"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	got := map[string]bool{}
	for _, p := range out.Providers {
		got[p.Name] = p.Available
		if p.Default != (p.Name == "sprites") {
			t.Errorf("provider %s default = %v", p.Name, p.Default)
		}
	}
	if !got["sprites"] || got["modal"] {
		t.Fatalf("availability = %v, want sprites available and modal not", got)
	}

	if strings.Contains(stderr.String(), "e2e-not-a-real-token") {
		t.Fatalf("token leaked to stderr: %s", stderr.String())
	}
}
