package mcp

import "github.com/perylemke/kaxa/internal/sandbox"

type providerInfo struct {
	Name         string               `json:"name"`
	Available    bool                 `json:"available"`
	Default      bool                 `json:"default"`
	Capabilities sandbox.Capabilities `json:"capabilities"`
}

type listProvidersIn struct{}

type listProvidersOut struct {
	Providers []providerInfo `json:"providers"`
}

type createIn struct {
	Provider string   `json:"provider,omitempty" jsonschema:"Provider name from list_providers. Defaults to KAXA_DEFAULT_PROVIDER."`
	Name     string   `json:"name" jsonschema:"Sandbox name. Lowercase letters, digits and hyphens."`
	Sync     bool     `json:"sync,omitempty" jsonschema:"Copy a local Git repository into the sandbox and install its dependencies."`
	Dir      string   `json:"dir,omitempty" jsonschema:"Directory inside the Git repository to copy. Requires sync. Defaults to the server working directory. Must be related to the server working directory."`
	Running  bool     `json:"running,omitempty" jsonschema:"Keep the sandbox running for one hour and serve a tmux terminal on its URL."`
	API      bool     `json:"api,omitempty" jsonschema:"Publish one HTTP service on the sandbox URL. Requires api_cmd."`
	APICmd   []string `json:"api_cmd,omitempty" jsonschema:"Argv of the HTTP service. The first item is the binary. The process must listen on 0.0.0.0 and on api_port."`
	APIPort  int      `json:"api_port,omitempty" jsonschema:"Port the HTTP service listens on. Defaults to 8000. Port 8080 is reserved when running is true."`
	APIEnv   []string `json:"api_env,omitempty" jsonschema:"Environment of the HTTP service. Each item is KEY=VALUE, or KEY to copy the value from the server environment so secrets stay out of the conversation. Values cannot contain commas. HOME, USER, PATH and KAXA_ names are reserved. Requires api."`
	Public   bool     `json:"public,omitempty" jsonschema:"Make the sandbox URL reachable without a provider account."`
}

type createOut struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	WorkDir  string `json:"work_dir,omitempty"`
	URL      string `json:"url,omitempty"`
	Auth     string `json:"auth,omitempty"`
}

type syncIn struct {
	Provider string `json:"provider,omitempty" jsonschema:"Provider name from list_providers. Defaults to KAXA_DEFAULT_PROVIDER."`
	Name     string `json:"name" jsonschema:"Name of a sandbox that was created with sync."`
	Dir      string `json:"dir,omitempty" jsonschema:"Directory inside the Git repository to copy. Defaults to the server working directory. Must be related to the server working directory."`
}

type syncOut struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	WorkDir  string `json:"work_dir,omitempty"`
}

type execIn struct {
	Provider string   `json:"provider,omitempty" jsonschema:"Provider name from list_providers. Defaults to KAXA_DEFAULT_PROVIDER."`
	Name     string   `json:"name" jsonschema:"Sandbox name."`
	Command  []string `json:"command" jsonschema:"Argv to run. The first item is the binary. It is never passed through a shell; use bash and -c explicitly if you need one."`
}

type execOut struct {
	ExitCode  int    `json:"exit_code"`
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	Truncated bool   `json:"truncated"`
}

type listIn struct {
	Provider string `json:"provider,omitempty" jsonschema:"Provider name from list_providers. Without it, every configured provider is listed."`
}

type sandboxInfo struct {
	ID       string   `json:"id"`
	Provider string   `json:"provider"`
	Status   string   `json:"status"`
	Labels   []string `json:"labels"`
}

type listOut struct {
	Sandboxes []sandboxInfo `json:"sandboxes"`
	Warnings  []string      `json:"warnings,omitempty"`
}

type nameIn struct {
	Provider string `json:"provider,omitempty" jsonschema:"Provider name from list_providers. Defaults to KAXA_DEFAULT_PROVIDER."`
	Name     string `json:"name" jsonschema:"Sandbox name."`
}

type inspectOut struct {
	ID           string   `json:"id"`
	Provider     string   `json:"provider"`
	Status       string   `json:"status"`
	Labels       []string `json:"labels"`
	Organization string   `json:"organization,omitempty"`
	URL          string   `json:"url,omitempty"`
	Auth         string   `json:"auth,omitempty"`
	CreatedAt    string   `json:"created_at,omitempty"`
	UpdatedAt    string   `json:"updated_at,omitempty"`
}

type actionOut struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Status   string `json:"status"`
}

type publishIn struct {
	Provider string   `json:"provider,omitempty" jsonschema:"Provider name from list_providers. Defaults to KAXA_DEFAULT_PROVIDER."`
	Name     string   `json:"name" jsonschema:"Sandbox name."`
	APICmd   []string `json:"api_cmd" jsonschema:"Argv of the HTTP service. The first item is the binary. The process must listen on 0.0.0.0 and on api_port."`
	APIPort  int      `json:"api_port,omitempty" jsonschema:"Port the HTTP service listens on. Defaults to 8000."`
	APIEnv   []string `json:"api_env,omitempty" jsonschema:"Environment of the HTTP service. Each item is KEY=VALUE, or KEY to copy the value from the server environment so secrets stay out of the conversation. Values cannot contain commas. HOME, USER, PATH and KAXA_ names are reserved."`
	WorkDir  string   `json:"work_dir,omitempty" jsonschema:"Absolute directory inside the sandbox to run the service from. Defaults to the sandbox home directory."`
}

type urlAuthIn struct {
	Provider string `json:"provider,omitempty" jsonschema:"Provider name from list_providers. Defaults to KAXA_DEFAULT_PROVIDER."`
	Name     string `json:"name" jsonschema:"Sandbox name."`
	Auth     string `json:"auth" jsonschema:"public lets anyone with the URL open it. sprite requires a Sprites account."`
}

type urlAuthOut struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Auth     string `json:"auth"`
	URL      string `json:"url,omitempty"`
}

type createSnapshotIn struct {
	Provider string `json:"provider,omitempty" jsonschema:"Provider name from list_providers. Defaults to KAXA_DEFAULT_PROVIDER."`
	Name     string `json:"name" jsonschema:"Sandbox name."`
	Comment  string `json:"comment,omitempty" jsonschema:"Short note stored with the snapshot. At most 256 bytes, no control characters. Do not put secrets in it."`
}

type snapshotInfo struct {
	ID        string `json:"id"`
	Sandbox   string `json:"sandbox"`
	Provider  string `json:"provider"`
	Comment   string `json:"comment,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

type listSnapshotsIn struct {
	Provider string `json:"provider,omitempty" jsonschema:"Provider name from list_providers. Without it, every configured provider is searched."`
	Name     string `json:"name,omitempty" jsonschema:"Name of a sandbox. Without it, the snapshots of every sandbox are listed."`
}

type listSnapshotsOut struct {
	Snapshots []snapshotInfo `json:"snapshots"`
	Warnings  []string       `json:"warnings,omitempty"`
}

type restoreSnapshotIn struct {
	Provider string `json:"provider,omitempty" jsonschema:"Provider name from list_providers. Defaults to KAXA_DEFAULT_PROVIDER."`
	Name     string `json:"name" jsonschema:"Name of the sandbox the snapshot was taken from."`
	Snapshot string `json:"snapshot" jsonschema:"Snapshot id from list_snapshots."`
	Target   string `json:"target,omitempty" jsonschema:"Name of the new sandbox. Required when the provider's snapshot_restore capability is new_sandbox, and rejected when it is in_place."`
}

type snapshotActionOut struct {
	// Sandbox holds the restored state: the source for in_place, the target for new_sandbox.
	Sandbox  string `json:"sandbox"`
	Snapshot string `json:"snapshot"`
	Provider string `json:"provider"`
	Status   string `json:"status"`
}

type deleteSnapshotIn struct {
	Provider string `json:"provider,omitempty" jsonschema:"Provider name from list_providers. Defaults to KAXA_DEFAULT_PROVIDER."`
	Name     string `json:"name" jsonschema:"Name of the sandbox the snapshot was taken from."`
	Snapshot string `json:"snapshot" jsonschema:"Snapshot id from list_snapshots."`
}
