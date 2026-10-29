// Package mcp serves the sandbox lifecycle to AI agents over the Model Context Protocol.
//
// Tools speak in sandbox terms. The provider (Sprites, Modal, and so on) is a
// parameter, never part of a tool name or a path, so one agent workflow works
// across providers. Stdout carries only the protocol; progress and logs go to stderr.
package mcp

import (
	"context"
	"io"
	"os"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/perylemke/kaxa/internal/config"
	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/usererr"
	"github.com/perylemke/kaxa/internal/version"
)

const instructions = `Run work in an isolated sandbox instead of on the host.
1. Call list_providers and pick a provider that is available.
2. create_sandbox with sync=true copies the local Git repository and installs its dependencies. The response work_dir is where the code lives inside the sandbox.
3. Run builds, tests and edits with exec (argv, no shell). exec starts above the repository, so run project commands from work_dir, for example ["bash", "-c", "cd <work_dir> && go test ./..."]. Files you change with local editor tools are not in the sandbox, and files changed in the sandbox are not copied back.
4. To add another Git repository to a sandbox that was created with sync, call sync_repository. It never replaces a directory.
5. To save a sandbox state, call create_snapshot; list_snapshots shows the ids, the sandbox and the provider of each snapshot. restore_snapshot either overwrites the sandbox (snapshot_restore in_place) or creates a new one named by target (new_sandbox).
6. Call destroy, delete_snapshot, or restore_snapshot on an in_place provider only when the user asks for it.`

// Options configures the server.
type Options struct {
	// Stderr receives progress lines and logs. Nil discards them.
	Stderr io.Writer
	// Root bounds which Git repositories sync may copy. Empty means the working directory.
	Root string
	// APIEnvAllow lists the server environment variables an agent may copy into
	// a service with a bare KEY in api_env. Empty means none.
	APIEnvAllow []string
	// LookupEnv reads the server environment. Nil means os.LookupEnv.
	LookupEnv func(string) (string, bool)
}

// EnvAPIEnvAllow names the variable that holds the comma-separated allowlist of
// server environment variables an agent may copy into a service.
const EnvAPIEnvAllow = "KAXA_MCP_API_ENV"

// ParseAPIEnvAllow splits the value of KAXA_MCP_API_ENV into variable names.
func ParseAPIEnvAllow(raw string) []string {
	var out []string
	for name := range strings.SplitSeq(raw, ",") {
		if name = strings.TrimSpace(name); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// New builds an MCP server with the sandbox tools registered.
func New(reg *registry.Registry, cfg config.Config, opts Options) (*sdk.Server, error) {
	svc, err := newService(reg, cfg, opts)
	if err != nil {
		return nil, err
	}
	srv := sdk.NewServer(&sdk.Implementation{Name: "kaxa", Title: "kaxa sandboxes", Version: version.Version}, &sdk.ServerOptions{
		Instructions: instructions,
	})
	svc.register(srv)
	return srv, nil
}

// Serve runs the server on stdin and stdout until the client disconnects or ctx ends.
func Serve(ctx context.Context, reg *registry.Registry, cfg config.Config, stderr io.Writer) error {
	srv, err := New(reg, cfg, Options{Stderr: stderr, APIEnvAllow: ParseAPIEnvAllow(os.Getenv(EnvAPIEnvAllow))})
	if err != nil {
		return err
	}
	return srv.Run(ctx, &sdk.StdioTransport{})
}

func (s *service) register(srv *sdk.Server) {
	addTool(s, srv, "list_providers", "List providers",
		"List the registered sandbox providers, whether each is available, and which optional features it supports (sync, setup, api, public_url, workspace_layout, snapshot). A provider with snapshot also reports snapshot_restore: in_place or new_sandbox.",
		readOnly, s.listProviders)
	addTool(s, srv, "create_sandbox", "Create a sandbox",
		"Create a sandbox. With sync it copies a local Git repository into the sandbox and installs its dependencies; a failed copy, install or API publish deletes the sandbox. Can take minutes.",
		additive, s.create)
	addTool(s, srv, "sync_repository", "Copy another repository into a sandbox",
		"Copy another local Git repository into a sandbox that was created with sync, then install its dependencies. It never replaces a directory: a repository already in the sandbox is an error. A failure keeps the sandbox. Can take minutes.",
		additive, s.syncRepository)
	addTool(s, srv, "exec", "Run a command",
		"Run an argv in a sandbox and return its stdout, stderr and exit code. A non-zero exit code is a result, not a tool error. Each stream is capped at 256 KiB.",
		openEnded, s.exec)
	addTool(s, srv, "list_sandboxes", "List sandboxes",
		"List the sandboxes of every configured provider, or only of the provider argument. A provider that fails is reported in warnings while the others are still listed.",
		readOnly, s.listSandboxes)
	addTool(s, srv, "inspect", "Inspect a sandbox",
		"Show status, labels, URL and URL auth of a sandbox.",
		readOnly, s.inspect)
	addTool(s, srv, "restart", "Restart a sandbox",
		"Restart a sandbox.",
		idempotent, s.restart)
	addTool(s, srv, "upgrade", "Upgrade a sandbox",
		"Upgrade a sandbox to the latest provider platform version.",
		idempotent, s.upgrade)
	addTool(s, srv, "destroy", "Destroy a sandbox",
		"Permanently delete a sandbox and its data. There is no undo. Call it only when the user asks.",
		destructive, s.destroy)
	addTool(s, srv, "create_snapshot", "Snapshot a sandbox",
		"Save the filesystem of a sandbox so it can be restored later. Only providers that list snapshot in their capabilities support it. The snapshot holds everything in the sandbox, including secrets written there.",
		additive, s.createSnapshot)
	addTool(s, srv, "list_snapshots", "List snapshots",
		"List snapshots with the provider and sandbox each was taken from. Without name, every snapshot of every configured provider is listed; name limits it to one sandbox and provider limits it to one provider. Providers that fail are reported in warnings. Use the snapshot ids with restore_snapshot and delete_snapshot.",
		readOnly, s.listSnapshots)
	addTool(s, srv, "restore_snapshot", "Restore a snapshot",
		"Restore a snapshot. The provider's snapshot_restore capability decides what happens: in_place overwrites the sandbox and discards its changes since the snapshot, so call it only when the user asks; new_sandbox creates the sandbox named by target and keeps the source.",
		destructive, s.restoreSnapshot)
	addTool(s, srv, "delete_snapshot", "Delete a snapshot",
		"Permanently delete a snapshot of a sandbox. There is no undo. Call it only when the user asks.",
		destructive, s.deleteSnapshot)
	addTool(s, srv, "publish_api", "Publish an HTTP service",
		"Start one HTTP service in an existing sandbox and route the sandbox URL to it. Only providers that list api in their capabilities support it.",
		additive, s.publishAPI)
	addTool(s, srv, "set_url_auth", "Set URL authentication",
		"Make the sandbox URL public or require a provider account. Only providers that list public_url in their capabilities support it.",
		idempotent, s.setURLAuth)
}

type toolKind int

const (
	readOnly toolKind = iota
	additive
	idempotent
	openEnded
	destructive
)

func (k toolKind) annotations(title string) *sdk.ToolAnnotations {
	no, yes := false, true
	a := &sdk.ToolAnnotations{Title: title, OpenWorldHint: &no}
	switch k {
	case readOnly:
		a.ReadOnlyHint = true
	case additive:
		a.DestructiveHint = &no
	case idempotent:
		a.DestructiveHint = &no
		a.IdempotentHint = true
	case openEnded:
		a.DestructiveHint = &yes
	case destructive:
		a.DestructiveHint = &yes
		a.IdempotentHint = true
	}
	return a
}

func addTool[In, Out any](s *service, srv *sdk.Server, name, title, description string, kind toolKind, fn func(context.Context, In) (Out, error)) {
	sdk.AddTool(srv, &sdk.Tool{
		Name:        name,
		Description: description,
		Annotations: kind.annotations(title),
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in In) (*sdk.CallToolResult, Out, error) {
		out, err := fn(ctx, in)
		if err != nil {
			var zero Out
			s.log.Warn("tool failed", "tool", name, "error", err)
			return nil, zero, withHint(lifecycle.WithAlternatives(s.reg, s.cfg, err))
		}
		return nil, out, nil
	})
}

// withHint appends the next step of a user error, so an agent sees it.
func withHint(err error) error {
	if ue, ok := usererr.As(err); ok && ue.Hint != "" {
		return &hintedError{err: err, hint: ue.Hint}
	}
	return err
}

type hintedError struct {
	err  error
	hint string
}

func (h *hintedError) Error() string { return h.err.Error() + " (" + h.hint + ")" }

func (h *hintedError) Unwrap() error { return h.err }
