package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/perylemke/kaxa/internal/config"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

func connect(t *testing.T, fp *fakeProvider, extra ...sandbox.Provider) *sdk.ClientSession {
	t.Helper()
	reg := registry.New()
	for _, p := range append([]sandbox.Provider{fp}, extra...) {
		if err := reg.Register(p); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	srv, err := New(reg, config.Config{Provider: fp.name}, Options{Root: t.TempDir(), Stderr: io.Discard})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ctx := context.Background()
	serverT, clientT := sdk.NewInMemoryTransports()
	serverSession, err := srv.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "v0"}, nil)
	session, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestServerListsEveryTool(t *testing.T) {
	t.Parallel()

	session := connect(t, &fakeProvider{name: testSprites})
	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
		if tool.Description == "" || tool.Annotations == nil || tool.InputSchema == nil {
			t.Errorf("tool %s is missing a description, annotations or schema", tool.Name)
		}
	}
	slices.Sort(names)
	want := []string{
		"create_sandbox", "destroy", "exec", "inspect", "list_providers",
		"list_sandboxes", "publish_api", "restart", "set_url_auth", "upgrade",
	}
	if !slices.Equal(names, want) {
		t.Fatalf("tools = %v, want %v", names, want)
	}
	for _, tool := range res.Tools {
		switch tool.Name {
		case "destroy":
			if tool.Annotations.DestructiveHint == nil || !*tool.Annotations.DestructiveHint {
				t.Errorf("destroy must be marked destructive")
			}
		case "list_providers", "list_sandboxes", "inspect":
			if !tool.Annotations.ReadOnlyHint {
				t.Errorf("%s must be read only", tool.Name)
			}
		}
	}
}

func call(t *testing.T, session *sdk.ClientSession, tool string, args map[string]any) *sdk.CallToolResult {
	t.Helper()
	res, err := session.CallTool(context.Background(), &sdk.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", tool, err)
	}
	return res
}

func decode[T any](t *testing.T, res *sdk.CallToolResult) T {
	t.Helper()
	if res.IsError {
		t.Fatalf("tool error: %s", text(res))
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal %s: %v", raw, err)
	}
	return out
}

func text(res *sdk.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*sdk.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func TestServerListProvidersMarksUnavailable(t *testing.T) {
	t.Parallel()

	session := connect(t, &fakeProvider{name: testSprites}, &fakeProvider{name: testModal, unavailable: true})
	out := decode[listProvidersOut](t, call(t, session, "list_providers", nil))
	if len(out.Providers) != 2 || out.Providers[0].Name != testModal || out.Providers[0].Available {
		t.Fatalf("providers = %+v", out.Providers)
	}
	if !out.Providers[1].Available || !out.Providers[1].Capabilities.Sync {
		t.Fatalf("sprites = %+v", out.Providers[1])
	}
}

func TestServerExecReturnsNonZeroExitAsResult(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testSprites, execFn: func(opts sandbox.ExecOptions) error {
		_, _ = io.WriteString(opts.Stdout, "hello\n")
		return &sandbox.ExitError{Code: 3}
	}}
	session := connect(t, fp)

	out := decode[execOut](t, call(t, session, "exec", map[string]any{"name": testBox, "command": []string{"sh"}}))
	if out.ExitCode != 3 || out.Stdout != "hello\n" {
		t.Fatalf("out = %+v", out)
	}
}

func TestServerCreateAndListRoundTrip(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testSprites, infos: []sandbox.Info{{ID: testBox, Provider: testSprites, Status: testWarm}}}
	session := connect(t, fp)

	created := decode[createOut](t, call(t, session, "create_sandbox", map[string]any{"name": testBox}))
	if created.ID != testBox || created.Provider != testSprites {
		t.Fatalf("created = %+v", created)
	}
	listed := decode[listOut](t, call(t, session, "list_sandboxes", nil))
	if len(listed.Sandboxes) != 1 || listed.Sandboxes[0].ID != testBox {
		t.Fatalf("listed = %+v", listed)
	}
}

func TestServerToolErrorCarriesHint(t *testing.T) {
	t.Parallel()

	session := connect(t, &fakeProvider{name: testSprites}, &fakeProvider{name: testModal, unavailable: true})
	res := call(t, session, "create_sandbox", map[string]any{"name": testBox, "provider": testModal})
	if !res.IsError {
		t.Fatal("expected a tool error")
	}
	msg := text(res)
	if !strings.Contains(msg, "modal provider is not available yet") || !strings.Contains(msg, "list_providers") {
		t.Fatalf("message = %q", msg)
	}
}

func TestServerRejectsMissingRequiredField(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testSprites}
	session := connect(t, fp)
	res := call(t, session, "destroy", map[string]any{})
	if !res.IsError || !strings.Contains(text(res), "name") {
		t.Fatalf("destroy without a name: isError=%v text=%q", res.IsError, text(res))
	}
	if len(fp.destroyed) != 0 {
		t.Fatalf("destroyed = %v", fp.destroyed)
	}
}

func TestWithHint(t *testing.T) {
	t.Parallel()

	if got := withHint(usererr.New("bad", "do this")).Error(); got != "bad (do this)" {
		t.Fatalf("hinted = %q", got)
	}
	plain := context.Canceled
	if got := withHint(plain); !errors.Is(got, plain) {
		t.Fatalf("plain error changed: %v", got)
	}
	if got := withHint(usererr.New("bad", "")).Error(); got != "bad" {
		t.Fatalf("no hint = %q", got)
	}
}
