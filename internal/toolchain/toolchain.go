// Package toolchain installs language toolchains and runs project setup steps
// inside a sandbox. It is provider neutral: a provider supplies a Runner and a
// Layout, and Setup does the rest.
package toolchain

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/perylemke/kaxa/internal/project"
)

const (
	argvVersion = "--version"
	pathScript  = "export PATH=\"$HOME/.local/bin:$HOME/.local/go/bin:$HOME/.cargo/bin:$PATH\"\n"
)

// Runner runs a command inside a sandbox.
type Runner interface {
	// Run runs argv in dir (empty means the default directory) with env added to
	// the environment, and returns its stdout. An error carries the stderr detail.
	Run(ctx context.Context, dir string, env, argv []string, stdin io.Reader) (string, error)
}

type installer struct {
	r Runner
	l Layout
}

func (i *installer) run(ctx context.Context, argv []string) (string, error) {
	return i.r.Run(ctx, "", nil, argv, nil)
}

func (i *installer) runStdin(ctx context.Context, argv []string, stdin io.Reader) (string, error) {
	return i.r.Run(ctx, "", nil, argv, stdin)
}

func (i *installer) runIn(ctx context.Context, dir string, env, argv []string, stdin io.Reader) (string, error) {
	return i.r.Run(ctx, dir, env, argv, stdin)
}

// Setup installs the toolchain each step needs, then runs the step's commands
// in workDir.
func Setup(ctx context.Context, r Runner, l Layout, workDir string, steps []project.Step) error {
	if len(steps) == 0 {
		return nil
	}
	i := &installer{r: r, l: l}
	if err := i.ensureUserPath(ctx); err != nil {
		return err
	}
	var arch archInfo
	var archReady bool
	for _, step := range steps {
		if err := ctx.Err(); err != nil {
			return err
		}
		bin, installed, err := i.ensureToolchain(ctx, step.Kind, &arch, &archReady)
		if err != nil {
			return err
		}
		if err := i.runProjectCommands(ctx, workDir, step, bin, installed); err != nil {
			return err
		}
	}
	return nil
}

// ChecksumMatches reports whether sha256sum output names the expected hex digest.
func ChecksumMatches(output, want string) bool {
	fields := strings.Fields(output)
	if len(fields) == 0 {
		return false
	}
	got, err := hex.DecodeString(fields[0])
	if err != nil {
		return false
	}
	expected, err := hex.DecodeString(want)
	if err != nil || len(got) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare(got, expected) == 1
}

func (i *installer) ensureUserPath(ctx context.Context) error {
	if _, err := i.run(ctx, []string{argvMkdir, "-p", i.l.configDir(), i.l.localBin()}); err != nil {
		return fmt.Errorf("configure path: %w", err)
	}
	if _, err := i.runStdin(ctx, []string{"tee", i.l.pathFile()}, strings.NewReader(pathScript)); err != nil {
		return fmt.Errorf("configure path: %w", err)
	}
	_, err := i.run(ctx, []string{"grep", "-F", "-q", i.l.pathFile(), i.l.bashrcFile()})
	if err == nil {
		return nil
	}
	line := "\n. " + i.l.pathFile() + "\n"
	if _, err := i.runStdin(ctx, []string{"tee", "-a", i.l.bashrcFile()}, strings.NewReader(line)); err != nil {
		return fmt.Errorf("configure path: %w", err)
	}
	return nil
}

func (i *installer) ensureToolchain(ctx context.Context, kind project.Kind, arch *archInfo, ready *bool) (string, bool, error) {
	probe, bin, err := i.l.toolPaths(kind)
	if err != nil {
		return "", false, err
	}
	if _, err := i.run(ctx, probe); err == nil {
		return probe[0], false, nil
	}
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if !*ready {
		got, err := i.detectArch(ctx)
		if err != nil {
			return "", false, err
		}
		*arch = got
		*ready = true
	}
	if err := i.installToolchain(ctx, kind, *arch); err != nil {
		return "", false, err
	}
	return bin, true, nil
}

func (i *installer) detectArch(ctx context.Context) (archInfo, error) {
	out, err := i.run(ctx, []string{"uname", "-m"})
	if err != nil {
		return archInfo{}, fmt.Errorf("detect arch: %w", err)
	}
	arch, err := archFromUname(out)
	if err != nil {
		return archInfo{}, fmt.Errorf("detect arch: %w", err)
	}
	return arch, nil
}

func (i *installer) installToolchain(ctx context.Context, kind project.Kind, arch archInfo) error {
	switch kind {
	case project.KindGo:
		return i.installGo(ctx, arch)
	case project.KindRust:
		return i.installRust(ctx, arch)
	case project.KindPython:
		return i.installUV(ctx, arch)
	case project.KindNode:
		return i.installNode(ctx, arch)
	case project.KindRuby:
		return i.installRuby(ctx, arch)
	case project.KindClojure:
		return i.installClojure(ctx, arch)
	default:
		return fmt.Errorf("unknown project kind %q", kind)
	}
}

func (i *installer) runProjectCommands(ctx context.Context, workDir string, step project.Step, bin string, installed bool) error {
	var env []string
	if installed {
		env = i.l.Env()
	}
	for _, argv := range step.Commands {
		if len(argv) == 0 {
			return fmt.Errorf("%s: empty command", step.Kind)
		}
		cmd := append([]string{}, argv...)
		cmd[0] = bin
		if err := i.runWithRetry(ctx, workDir, env, cmd); err != nil {
			return fmt.Errorf("%s: %w", step.Kind, err)
		}
	}
	return nil
}

func (i *installer) installGo(ctx context.Context, arch archInfo) error {
	asset := goAsset(arch)
	if err := i.downloadVerified(ctx, asset); err != nil {
		return installFailed("go", err)
	}
	if _, err := i.run(ctx, []string{argvMkdir, "-p", i.l.localPrefix()}); err != nil {
		return installFailed("go", err)
	}
	if _, err := i.run(ctx, []string{argvTar, tarGzip, asset.archive, "-C", i.l.localPrefix()}); err != nil {
		_ = i.remove(ctx, asset.archive)
		return installFailed("go", err)
	}
	if err := i.remove(ctx, asset.archive); err != nil {
		return installFailed("go", err)
	}
	return nil
}

func (i *installer) installNode(ctx context.Context, arch archInfo) error {
	asset := nodeAsset(arch)
	if err := i.downloadVerified(ctx, asset); err != nil {
		return installFailed("node", err)
	}
	if err := i.extractStrip(ctx, asset.archive, i.l.nodePrefix()); err != nil {
		return installFailed("node", err)
	}
	links := [][2]string{
		{i.l.nodePrefix() + "/bin/node", i.l.localBin() + "/node"},
		{i.l.nodePrefix() + "/bin/npm", i.l.npmBin()},
		{i.l.nodePrefix() + "/bin/npx", i.l.localBin() + "/npx"},
	}
	for _, link := range links {
		if _, err := i.run(ctx, []string{"ln", "-sfn", link[0], link[1]}); err != nil {
			return installFailed("node", err)
		}
	}
	return nil
}

func (i *installer) installUV(ctx context.Context, arch archInfo) error {
	asset := uvAsset(arch)
	if err := i.downloadVerified(ctx, asset); err != nil {
		return installFailed("uv", err)
	}
	const extractDir = "/tmp/kaxa-uv"
	if err := i.extractStrip(ctx, asset.archive, extractDir); err != nil {
		return installFailed("uv", err)
	}
	if _, err := i.run(ctx, []string{"mv", extractDir + "/uv", i.l.uvBin()}); err != nil {
		return installFailed("uv", err)
	}
	if _, err := i.run(ctx, []string{"mv", extractDir + "/uvx", i.l.localBin() + "/uvx"}); err != nil {
		return installFailed("uv", err)
	}
	if _, err := i.run(ctx, []string{"chmod", "755", i.l.uvBin(), i.l.localBin() + "/uvx"}); err != nil {
		return installFailed("uv", err)
	}
	if err := i.remove(ctx, extractDir); err != nil {
		return installFailed("uv", err)
	}
	return nil
}

func (i *installer) installRust(ctx context.Context, arch archInfo) error {
	asset := rustupAsset(arch)
	if err := i.downloadVerified(ctx, asset); err != nil {
		return installFailed("rust", err)
	}
	if _, err := i.run(ctx, []string{"chmod", "755", asset.archive}); err != nil {
		return installFailed("rust", err)
	}
	argv := []string{asset.archive, "-y", "--no-modify-path", "--profile", "minimal", "--default-toolchain", "stable"}
	if _, err := i.runIn(ctx, "", i.l.Env(), argv, nil); err != nil {
		_ = i.remove(ctx, asset.archive)
		return installFailed("rust", err)
	}
	if err := i.remove(ctx, asset.archive); err != nil {
		return installFailed("rust", err)
	}
	return nil
}

func (i *installer) downloadVerified(ctx context.Context, asset toolAsset) error {
	if _, err := i.run(ctx, []string{"curl", "-fsSL", "-o", asset.archive, asset.url}); err != nil {
		return fmt.Errorf("download: %w", err)
	}
	sumOut, err := i.run(ctx, []string{"sha256sum", asset.archive})
	if err != nil {
		_ = i.remove(ctx, asset.archive)
		return fmt.Errorf("checksum: %w", err)
	}
	if !ChecksumMatches(sumOut, asset.sha256) {
		_ = i.remove(ctx, asset.archive)
		return errors.New("checksum: archive hash does not match")
	}
	return nil
}

func (i *installer) extractStrip(ctx context.Context, archive, dest string) error {
	if _, err := i.run(ctx, []string{argvMkdir, "-p", dest}); err != nil {
		return err
	}
	if _, err := i.run(ctx, []string{argvTar, tarGzip, archive, "-C", dest, "--strip-components=1"}); err != nil {
		_ = i.remove(ctx, archive)
		return err
	}
	return i.remove(ctx, archive)
}

func (i *installer) remove(ctx context.Context, path string) error {
	_, err := i.run(ctx, []string{"rm", "-rf", path})
	return err
}

func installFailed(tool string, err error) error {
	return fmt.Errorf("install %s: %w", tool, err)
}
