package cli_test

import (
	"context"
	"errors"
	"testing"

	"github.com/perylemke/kaxa/internal/cli"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

const plainName = "plain"

// coreProvider implements only the required sandbox.Provider methods.
type coreProvider struct{ name string }

func (c coreProvider) Name() string { return c.name }
func (coreProvider) Create(_ context.Context, spec sandbox.Spec) (sandbox.ID, error) {
	return sandbox.ID(spec.Name), nil
}
func (coreProvider) List(context.Context) ([]sandbox.Info, error) { return nil, nil }
func (coreProvider) Inspect(context.Context, sandbox.ID) (sandbox.Detail, error) {
	return sandbox.Detail{}, nil
}
func (coreProvider) Exec(context.Context, sandbox.ID, sandbox.ExecOptions) error { return nil }
func (coreProvider) Destroy(context.Context, sandbox.ID) error                   { return nil }

func TestRestartAndUpgradeRejectedWithoutCapability(t *testing.T) {
	tests := []struct {
		name    string
		command string
		feature sandbox.Feature
	}{
		{"restart", testCmdRestart, sandbox.FeatureRestart},
		{"upgrade", testCmdUpgrade, sandbox.FeatureUpgrade},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newCreateCmd(t, coreProvider{name: plainName})
			cmd.SetArgs([]string{testProviderFlag, plainName, tt.command, testBoxName})

			err := cmd.Execute()
			var ue *sandbox.UnsupportedError
			if !errors.Is(err, sandbox.ErrUnsupported) || !errors.As(err, &ue) || ue.Feature != tt.feature {
				t.Fatalf("err = %v, want unsupported %s", err, tt.feature)
			}
		})
	}
}

func TestUnsupportedErrorHintNamesAlternatives(t *testing.T) {
	reg := registry.New()
	for _, p := range []sandbox.Provider{&fakeProvider{name: testSpritesName}, coreProvider{name: plainName}} {
		if err := reg.Register(p); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	cmd := cli.NewRootCmd(reg)
	cmd.SetArgs([]string{testProviderFlag, plainName, testCmdRestart, testBoxName})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("restart on plain provider should fail")
	}
	ue, ok := usererr.As(err)
	if !ok || ue.Message != "restart is not available for plain" {
		t.Fatalf("err = %#v", err)
	}
}
