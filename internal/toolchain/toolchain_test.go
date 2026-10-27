package toolchain

import (
	"context"
	"errors"
	"io"
	"path"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/project"
)

const (
	testUserSprite = "sprite"
	testUserModal  = "modal"
	testAllPkgs    = "./..."
)

var testLayout = Layout{Home: "/home/sprite", User: testUserSprite}

const testWorkDir = "/home/sprite/workspace/api"

const projectBuild = "build"

func TestToolAssetsPinChecksums(t *testing.T) {
	t.Parallel()

	amd := archInfo{goArch: archAMD64, node: "x64", triple: "x86_64-unknown-linux-gnu"}
	arm := archInfo{goArch: archARM64, node: archARM64, triple: "aarch64-unknown-linux-gnu"}
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

const (
	toolBundle     = "bundle"
	toolClojure    = "clojure"
	tarExtract     = "-xzf"
	tarStrip       = "--strip-components=1"
	rubyArchive    = "/tmp/kaxa-ruby.tar.gz"
	jdkArchive     = "/tmp/kaxa-jdk.tar.gz"
	clojureArchive = "/tmp/kaxa-clojure.tar.gz"
)

var (
	testAMD64 = archInfo{goArch: archAMD64, node: "x64", triple: "x86_64-unknown-linux-gnu"}
	testARM64 = archInfo{goArch: archARM64, node: archARM64, triple: "aarch64-unknown-linux-gnu"}
)

func TestRubyJDKClojureAssetsPinChecksums(t *testing.T) {
	t.Parallel()

	if got := rubyAsset(testAMD64); got.sha256 != rubyAMD64SHA || !strings.HasSuffix(got.url, "/3.4.12/ruby-3.4.12.x86_64_linux.tar.gz") {
		t.Fatalf("ruby amd64 = %#v", got)
	}
	if got := rubyAsset(testARM64); got.sha256 != rubyARM64SHA || !strings.HasSuffix(got.url, "/3.4.12/ruby-3.4.12.arm64_linux.tar.gz") {
		t.Fatalf("ruby arm64 = %#v", got)
	}
	if got := jdkAsset(testAMD64); got.sha256 != jdkAMD64SHA || !strings.HasSuffix(got.url, "jdk-21.0.9%2B10/OpenJDK21U-jdk_x64_linux_hotspot_21.0.9_10.tar.gz") {
		t.Fatalf("jdk amd64 = %#v", got)
	}
	if got := jdkAsset(testARM64); got.sha256 != jdkARM64SHA || !strings.Contains(got.url, "jdk_aarch64_linux_hotspot") {
		t.Fatalf("jdk arm64 = %#v", got)
	}
	if got := clojureAsset(); got.sha256 != clojureSHA || !strings.HasSuffix(got.url, "/1.12.6.1673/clojure-tools-1.12.6.1673.tar.gz") {
		t.Fatalf("clojure = %#v", got)
	}
}

func TestSetupProjectsInstallsMissingRuby(t *testing.T) {
	t.Parallel()

	client := &setupStub{
		missing: map[string]bool{toolBundle: true},
		sums:    map[string]string{rubyArchive: rubyAMD64SHA},
	}
	err := Setup(context.Background(), client, testLayout, testWorkDir, []project.Step{{
		Kind:     project.KindRuby,
		Commands: [][]string{{toolBundle, "install", "--frozen"}},
	}})
	if err != nil {
		t.Fatalf("SetupProjects: %v", err)
	}
	mustSee(t, client.calls, "curl", "-fsSL", "-o", rubyArchive, rubyAsset(testAMD64).url)
	mustSee(t, client.calls, "tar", tarExtract, rubyArchive, "-C", testLayout.rubyPrefix(), tarStrip)
	for _, tool := range []string{"ruby", "gem", toolBundle} {
		mustSee(t, client.calls, "ln", "-sfn", testLayout.rubyPrefix()+"/bin/"+tool, testLayout.localBin()+"/"+tool)
	}
	install := lastCallWith(client.calls, 3, 1, "install")
	if install.argv[0] != testLayout.bundleBin() || install.argv[2] != "--frozen" || install.dir != testWorkDir || len(install.env) == 0 {
		t.Fatalf("install = %#v", install)
	}
}

func TestSetupProjectsInstallsMissingClojureAndJDK(t *testing.T) {
	t.Parallel()

	client := &setupStub{
		missing: map[string]bool{toolClojure: true, "java": true},
		sums:    map[string]string{jdkArchive: jdkAMD64SHA, clojureArchive: clojureSHA},
	}
	err := Setup(context.Background(), client, testLayout, testWorkDir, []project.Step{{
		Kind:     project.KindClojure,
		Commands: [][]string{{toolClojure, "-P"}},
	}})
	if err != nil {
		t.Fatalf("SetupProjects: %v", err)
	}
	mustSee(t, client.calls, "tar", tarExtract, jdkArchive, "-C", testLayout.jdkPrefix(), tarStrip)
	mustSee(t, client.calls, "ln", "-sfn", testLayout.jdkPrefix()+"/bin/java", testLayout.javaBin())
	mustSee(t, client.calls, "tar", tarExtract, clojureArchive, "-C", testLayout.clojurePrefix(), tarStrip)
	for _, jar := range []string{"clojure-tools-1.12.6.1673.jar", "exec.jar"} {
		mustSee(t, client.calls, "mv", testLayout.clojurePrefix()+"/"+jar, testLayout.clojurePrefix()+"/libexec/"+jar)
	}
	mustSee(t, client.calls, "sed", "-i", "s|^install_dir=PREFIX$|install_dir="+testLayout.clojurePrefix()+"|", testLayout.clojurePrefix()+"/clojure")
	mustSee(t, client.calls, "ln", "-sfn", testLayout.clojurePrefix()+"/clojure", testLayout.clojureBin())
	prep := lastCallWith(client.calls, 2, 1, "-P")
	if prep.argv[0] != testLayout.clojureBin() || prep.dir != testWorkDir || len(prep.env) == 0 {
		t.Fatalf("prep = %#v", prep)
	}
}

func TestSetupProjectsClojureKeepsExistingJava(t *testing.T) {
	t.Parallel()

	client := &setupStub{
		missing: map[string]bool{toolClojure: true},
		sums:    map[string]string{clojureArchive: clojureSHA},
	}
	err := Setup(context.Background(), client, testLayout, testWorkDir, []project.Step{{
		Kind:     project.KindClojure,
		Commands: [][]string{{toolClojure, "-P"}},
	}})
	if err != nil {
		t.Fatalf("SetupProjects: %v", err)
	}
	if sawArgv(client.calls, []string{"tar", tarExtract, jdkArchive, "-C", testLayout.jdkPrefix(), tarStrip}) {
		t.Fatalf("installed a JDK although java exists: %#v", argvOf(client.calls))
	}
}

func TestSetupProjectsRejectsBadRubyChecksum(t *testing.T) {
	t.Parallel()

	client := &setupStub{
		missing: map[string]bool{toolBundle: true},
		sums:    map[string]string{rubyArchive: strings.Repeat("0", 64)},
	}
	err := Setup(context.Background(), client, testLayout, testWorkDir, []project.Step{{
		Kind:     project.KindRuby,
		Commands: [][]string{{toolBundle, "install"}},
	}})
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("err = %v", err)
	}
	if sawCommand(client.calls, "tar") {
		t.Fatal("extracted an archive with a bad checksum")
	}
}

// mustSee fails unless a remote call ran exactly argv.
func mustSee(t *testing.T, calls []remoteCall, argv ...string) {
	t.Helper()
	if !sawArgv(calls, argv) {
		t.Fatalf("missing call %v in %#v", argv, argvOf(calls))
	}
}

// lastCallWith returns the last call with n args whose argv[idx] equals want.
func lastCallWith(calls []remoteCall, n, idx int, want string) remoteCall {
	var found remoteCall
	for _, call := range calls {
		if len(call.argv) == n && call.argv[idx] == want {
			found = call
		}
	}
	return found
}

func TestSetupProjectsInstallsMissingGo(t *testing.T) {
	t.Parallel()

	client := &setupStub{missing: map[string]bool{"go": true}}
	err := Setup(context.Background(), client, testLayout, testWorkDir, []project.Step{{
		Kind:     project.KindGo,
		Commands: [][]string{{"go", projectBuild, testAllPkgs}},
	}})
	if err != nil {
		t.Fatalf("SetupProjects: %v", err)
	}
	if !sawArgv(client.calls, []string{"curl", "-fsSL", "-o", "/tmp/kaxa-go.tar.gz", goAsset(archInfo{goArch: archAMD64, node: "x64", triple: "x86_64-unknown-linux-gnu"}).url}) {
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
	if !found || build.argv[0] != testLayout.goBin() || build.dir != testWorkDir || len(build.env) == 0 {
		t.Fatalf("build = %#v", build)
	}
	if !sawStdin(client.calls, pathScript) {
		t.Fatal("path script was not written")
	}
}

func TestSetupProjectsUsesExistingGo(t *testing.T) {
	t.Parallel()

	client := &setupStub{}
	err := Setup(context.Background(), client, testLayout, testWorkDir, []project.Step{{
		Kind:     project.KindGo,
		Commands: [][]string{{"go", projectBuild, testAllPkgs}},
	}})
	if err != nil {
		t.Fatalf("SetupProjects: %v", err)
	}
	for _, call := range client.calls {
		if call.argv[0] == "curl" {
			t.Fatalf("downloaded a toolchain: %#v", call.argv)
		}
		if len(call.argv) == 3 && call.argv[1] == projectBuild {
			if call.argv[0] != "go" || call.dir != testWorkDir || call.env != nil {
				t.Fatalf("build = %#v", call)
			}
		}
	}
}

func TestSetupProjectsRejectsBadChecksum(t *testing.T) {
	t.Parallel()

	client := &setupStub{missing: map[string]bool{"go": true}, badSum: true}
	err := Setup(context.Background(), client, testLayout, testWorkDir, []project.Step{{
		Kind:     project.KindGo,
		Commands: [][]string{{"go", projectBuild, testAllPkgs}},
	}})
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("err = %v", err)
	}
	if sawCommand(client.calls, "tar") {
		t.Fatal("extracted an archive with a bad checksum")
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
	sums    map[string]string
}

func (s *setupStub) Run(_ context.Context, dir string, env, argv []string, stdin io.Reader) (string, error) {
	var stdout strings.Builder
	body := ""
	if stdin != nil {
		buf, err := io.ReadAll(stdin)
		if err != nil {
			return "", err
		}
		body = string(buf)
	}
	call := remoteCall{dir: dir, env: append([]string(nil), env...), argv: append([]string(nil), argv...), stdin: body}
	s.calls = append(s.calls, call)
	switch {
	case len(argv) > 0 && s.missing[argv[0]] && !strings.HasPrefix(argv[0], "/"):
		return "", errors.New("missing tool")
	case argv[0] == "uname":
		stdout.WriteString("x86_64\n")
	case argv[0] == "sha256sum":
		sum := goAMD64SHA
		if pinned, ok := s.sums[argv[len(argv)-1]]; ok {
			sum = pinned
		}
		if s.badSum {
			sum = "0000000000000000000000000000000000000000000000000000000000000000"
		}
		stdout.WriteString(sum + "  " + argv[len(argv)-1] + "\n")
	case argv[0] == "grep":
		return "", errors.New("exit status 1")
	}
	return stdout.String(), nil
}

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

func TestLayoutDerivesPathsFromHome(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		layout Layout
		home   string
		user   string
	}{
		{testUserSprite, Layout{Home: "/home/sprite", User: testUserSprite}, "/home/sprite", testUserSprite},
		{testUserModal, Layout{Home: "/home/modal", User: testUserModal}, "/home/modal", testUserModal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.layout.localBin(); got != tt.home+"/.local/bin" {
				t.Fatalf("localBin = %q", got)
			}
			if got := tt.layout.goBin(); got != tt.home+"/.local/go/bin/go" {
				t.Fatalf("goBin = %q", got)
			}
			env := tt.layout.Env()
			if env[0] != "HOME="+tt.home || env[1] != "USER="+tt.user || !strings.Contains(env[2], tt.home+"/.cargo/bin") {
				t.Fatalf("env = %v", env)
			}
		})
	}
}

func TestSetupInstallsAsConfiguredUser(t *testing.T) {
	t.Parallel()

	client := &setupStub{missing: map[string]bool{"go": true}}
	layout := Layout{Home: "/home/modal", User: testUserModal}
	err := Setup(context.Background(), client, layout, "/home/modal/workspace/api", []project.Step{{
		Kind:     project.KindGo,
		Commands: [][]string{{"go", projectBuild, testAllPkgs}},
	}})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	mustSee(t, client.calls, "tar", tarExtract, "/tmp/kaxa-go.tar.gz", "-C", "/home/modal/.local")
	build := lastCallWith(client.calls, 3, 1, projectBuild)
	if build.argv[0] != "/home/modal/.local/go/bin/go" || build.dir != "/home/modal/workspace/api" || build.env[0] != "HOME=/home/modal" {
		t.Fatalf("build = %#v", build)
	}
	if sawStdin(client.calls, "/home/sprite") {
		t.Fatal("sprite paths leaked into a modal layout")
	}
}

func TestRustupInitKeepsItsOwnFileName(t *testing.T) {
	t.Parallel()

	// rustup-init acts as a rustup proxy under any other name and fails with
	// "unknown proxy name", so the downloaded file must be called rustup-init.
	if got := path.Base(rustupAsset(archInfo{goArch: archAMD64}).archive); got != "rustup-init" {
		t.Fatalf("rustup-init archive is named %q", got)
	}
}
