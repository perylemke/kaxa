package web

import (
	"context"
	"io"

	"github.com/perylemke/kaxa/internal/sandbox"
)

type fakeProvider struct {
	name        string
	unavailable bool
	list        []sandbox.Info
	listErr     error
	detail      sandbox.Detail
	inspectErr  error
	createErr   error
	created     sandbox.Spec
	destroyed   sandbox.ID
	destroyErr  error
	restarted   sandbox.ID
	restartErr  error
	upgraded    sandbox.ID
	upgradeErr  error
	auth        string
	authErr     error
	preview     string
}

func (f *fakeProvider) Name() string { return f.name }

func (f *fakeProvider) Available() bool { return !f.unavailable }

func (f *fakeProvider) Create(_ context.Context, spec sandbox.Spec) (sandbox.ID, error) {
	f.created = spec
	if f.createErr != nil {
		return "", f.createErr
	}
	return sandbox.ID(spec.Name), nil
}

func (f *fakeProvider) List(context.Context) ([]sandbox.Info, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.list, nil
}

func (f *fakeProvider) Inspect(_ context.Context, id sandbox.ID) (sandbox.Detail, error) {
	if f.inspectErr != nil {
		return sandbox.Detail{}, f.inspectErr
	}
	if f.detail.ID != "" {
		return f.detail, nil
	}
	return sandbox.Detail{ID: id, Provider: f.name, Status: testStatus, URL: "https://example.test/" + string(id)}, nil
}

func (f *fakeProvider) Exec(context.Context, sandbox.ID, sandbox.ExecOptions) error { return nil }

func (f *fakeProvider) Destroy(_ context.Context, id sandbox.ID) error {
	f.destroyed = id
	return f.destroyErr
}

type capable struct{ *fakeProvider }

func (c capable) Restart(_ context.Context, id sandbox.ID) error {
	c.restarted = id
	return c.restartErr
}

func (c capable) Upgrade(_ context.Context, id sandbox.ID) error {
	c.upgraded = id
	return c.upgradeErr
}

func (c capable) SetURLAuth(_ context.Context, _ sandbox.ID, auth string) error {
	c.auth = auth
	return c.authErr
}

func (capable) SupportsRunning() {}

func (c capable) TakePreviewToken(sandbox.ID) string {
	token := c.preview
	c.preview = ""
	return token
}

func (capable) HomeDir() string { return "/home/lab" }

func (capable) RepoDir(repo string) (string, error) {
	return "/home/lab/workspace/" + repo, nil
}

func (capable) PrepareWorkspace(context.Context, sandbox.ID) error { return nil }

func (capable) SyncWorkspace(context.Context, sandbox.ID, io.Reader) error { return nil }

func (capable) PublishAPI(context.Context, sandbox.ID, string, int, []string, []string) error {
	return nil
}

func (capable) SupportsPublicPreview() {}
