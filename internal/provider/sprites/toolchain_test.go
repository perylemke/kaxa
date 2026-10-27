package sprites

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	spritesdk "github.com/superfly/sprites-go"

	"github.com/perylemke/kaxa/internal/project"
)

const projectBuild = "build"

func TestToolAssetsPinChecksums(t *testing.T) {
	t.Parallel()

	amd := spriteArch{goArch: archAMD64, node: "x64", triple: "x86_64-unknown-linux-gnu"}
	arm := spriteArch{goArch: archARM64, node: archARM64, triple: "aarch64-unknown-linux-gnu"}
	if goAsset(amd).sha256 != goAMD64SHA || !strings.Contains(goAsset(amd).url, "go1.27.3.linux-amd64.tar.gz") {
		t.Fatalf("go amd64 = %#v", goAsset(amd))
	}
	if goAsset(arm).sha256 != goARM64SHA || !strings.Contains(nodeAsset(arm).url, "linux-arm64.tar.gz") {
		t.Fatalf("arm assets go=%#v node=%#v", goAsset(arm), nodeAsset(arm))
	}
	if uvAsset(amd).sha256 != uvAMD64SHA || rustupAsset(arm).sha256 != rustARM64SHA {
		t.Fatalf("uv=%#v rust=%#v", uvAsset(amd), rustupAsset(arm))
	}
}

func TestSetupProjectsInstallsMissingGo(t *testing.T) {
	t.Parallel()

	client := &setupStub{missing: map[string]bool{"go": true}}
	p := NewWithClient(client, "token")
	err := p.SetupProjects(context.Background(), "box", "api", []project.Step{{
		Kind:     project.KindGo,
		Commands: [][]string{{"go", projectBuild, "./..."}},
	}})
	if err != nil {
		t.Fatalf("SetupProjects: %v", err)
	}
	if !sawArgv(client.calls, []string{"curl", "-fsSL", "-o", "/tmp/kaxa-go.tar.gz", goAsset(spriteArch{goArch: archAMD64, node: "x64", triple: "x86_64-unknown-linux-gnu"}).url}) {
		t.Fatalf("curl calls = %#v", argvOf(client.calls))
	}
	var build remoteCall
	var found bool
	for _, call := range client.calls {
		if len(call.argv) == 3 && call.argv[1] == projectBuild {
			build = call
			found = true
		}
	}
	if !found || build.argv[0] != goBin || build.dir != workspaceDir+"/api" || len(build.env) == 0 {
		t.Fatalf("build = %#v", build)
	}
	if !sawStdin(client.calls, pathScript) {
		t.Fatal("path script was not written")
	}
}

func TestSetupProjectsUsesExistingGo(t *testing.T) {
	t.Parallel()

	client := &setupStub{}
	p := NewWithClient(client, "token")
	err := p.SetupProjects(context.Background(), "box", "api", []project.Step{{
		Kind:     project.KindGo,
		Commands: [][]string{{"go", projectBuild, "./..."}},
	}})
	if err != nil {
		t.Fatalf("SetupProjects: %v", err)
	}
	for _, call := range client.calls {
		if call.argv[0] == "curl" {
			t.Fatalf("downloaded a toolchain: %#v", call.argv)
		}
		if len(call.argv) == 3 && call.argv[1] == projectBuild {
			if call.argv[0] != "go" || call.dir != workspaceDir+"/api" || call.env != nil {
				t.Fatalf("build = %#v", call)
			}
		}
	}
}

func TestSetupProjectsRejectsBadChecksum(t *testing.T) {
	t.Parallel()

	client := &setupStub{missing: map[string]bool{"go": true}, badSum: true}
	p := NewWithClient(client, "token")
	err := p.SetupProjects(context.Background(), "box", "api", []project.Step{{
		Kind:     project.KindGo,
		Commands: [][]string{{"go", projectBuild, "./..."}},
	}})
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("err = %v", err)
	}
	if sawCommand(client.calls, "tar") {
		t.Fatal("extracted an archive with a bad checksum")
	}
}

func TestSetupProjectsRejectsRepoDir(t *testing.T) {
	t.Parallel()

	client := &setupStub{}
	p := NewWithClient(client, "token")
	if err := p.SetupProjects(context.Background(), "box", "..", nil); err == nil {
		t.Fatal("expected invalid directory error")
	}
	if len(client.calls) != 0 {
		t.Fatalf("calls = %#v", client.calls)
	}
}

type remoteCall struct {
	dir   string
	env   []string
	argv  []string
	stdin string
}

type setupStub struct {
	calls   []remoteCall
	missing map[string]bool
	badSum  bool
}

func (s *setupStub) CreateSprite(context.Context, string, []string) (*spritesdk.Sprite, error) {
	return &spritesdk.Sprite{}, nil
}

func (s *setupStub) GetSprite(context.Context, string) (*spritesdk.Sprite, error) {
	return &spritesdk.Sprite{}, nil
}

func (s *setupStub) ListEntries(context.Context, string) ([]SpriteListEntry, error) {
	return nil, nil
}

func (s *setupStub) UpgradeSprite(context.Context, string) error { return nil }

func (s *setupStub) RestartSprite(context.Context, string) error { return nil }

func (s *setupStub) Run(ctx context.Context, name string, argv []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return s.RunIn(ctx, name, "", nil, argv, stdin, stdout, stderr)
}

func (s *setupStub) RunIn(_ context.Context, _, dir string, env, argv []string, stdin io.Reader, stdout, _ io.Writer) error {
	body := ""
	if stdin != nil {
		buf, err := io.ReadAll(stdin)
		if err != nil {
			return err
		}
		body = string(buf)
	}
	call := remoteCall{dir: dir, env: append([]string(nil), env...), argv: append([]string(nil), argv...), stdin: body}
	s.calls = append(s.calls, call)
	switch {
	case len(argv) > 0 && s.missing[argv[0]] && !strings.HasPrefix(argv[0], "/"):
		return errors.New("missing tool")
	case argv[0] == "uname":
		_, _ = io.WriteString(stdout, "x86_64\n")
	case argv[0] == "sha256sum":
		sum := goAMD64SHA
		if s.badSum {
			sum = "0000000000000000000000000000000000000000000000000000000000000000"
		}
		_, _ = io.WriteString(stdout, sum+"  "+argv[len(argv)-1]+"\n")
	case argv[0] == "grep":
		return errors.New("exit status 1")
	}
	return nil
}

func (s *setupStub) DestroySprite(context.Context, string) error { return nil }

func (s *setupStub) Sprite(string) *spritesdk.Sprite { return nil }

func (s *setupStub) Close() error { return nil }

func sawArgv(calls []remoteCall, want []string) bool {
	for _, call := range calls {
		if len(call.argv) != len(want) {
			continue
		}
		match := true
		for i := range want {
			if call.argv[i] != want[i] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func sawCommand(calls []remoteCall, name string) bool {
	for _, call := range calls {
		if len(call.argv) > 0 && call.argv[0] == name {
			return true
		}
	}
	return false
}

func sawStdin(calls []remoteCall, text string) bool {
	for _, call := range calls {
		if strings.Contains(call.stdin, text) {
			return true
		}
	}
	return false
}

func argvOf(calls []remoteCall) [][]string {
	out := make([][]string, 0, len(calls))
	for _, call := range calls {
		out = append(out, call.argv)
	}
	return out
}
