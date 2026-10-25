package sprites

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/perylemke/sbox/internal/project"
	"github.com/perylemke/sbox/internal/sandbox"
	"github.com/perylemke/sbox/internal/usererr"
)

const (
	spriteHome  = "/home/sprite"
	localPrefix = spriteHome + "/.local"
	localBin    = localPrefix + "/bin"
	configDir   = spriteHome + "/.config/sbox"
	pathFile    = configDir + "/path.sh"
	bashrcFile  = spriteHome + "/.bashrc"
	goBin       = localPrefix + "/go/bin/go"
	cargoBin    = spriteHome + "/.cargo/bin/cargo"
	uvBin       = localBin + "/uv"
	npmBin      = localBin + "/npm"
	nodePrefix  = localPrefix + "/node"
	pathScript  = "export PATH=\"$HOME/.local/bin:$HOME/.local/go/bin:$HOME/.cargo/bin:$PATH\"\n"
)

// SetupProjects installs missing toolchains and runs each detected project command.
func (p *Provider) SetupProjects(ctx context.Context, id sandbox.ID, repoName string, steps []project.Step) error {
	if err := validateName(string(id)); err != nil {
		return err
	}
	if err := validateRepoDir(repoName); err != nil {
		return err
	}
	if len(steps) == 0 {
		return nil
	}
	c, token, err := p.clientAndToken()
	if err != nil {
		return err
	}
	name := string(id)
	if err := ensureUserPath(ctx, c, name, token); err != nil {
		return err
	}
	workDir := workspaceDir + "/" + repoName
	var arch spriteArch
	var archReady bool
	for _, step := range steps {
		if err := ctx.Err(); err != nil {
			return err
		}
		bin, installed, err := ensureToolchain(ctx, c, name, token, step.Kind, &arch, &archReady)
		if err != nil {
			return err
		}
		if err := runProjectCommands(ctx, c, name, token, workDir, step, bin, installed); err != nil {
			return err
		}
	}
	return nil
}

func validateRepoDir(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) || strings.ContainsRune(name, 0) {
		return usererr.New(fmt.Sprintf("invalid repository directory %q", name), "use a single path segment")
	}
	return nil
}

func ensureUserPath(ctx context.Context, c sdkClient, name, token string) error {
	if _, err := runRemote(ctx, c, name, token, []string{argvMkdir, "-p", configDir, localBin}); err != nil {
		return fmt.Errorf("configure path: %w", err)
	}
	if _, err := runRemoteStdin(ctx, c, name, token, []string{"tee", pathFile}, strings.NewReader(pathScript)); err != nil {
		return fmt.Errorf("configure path: %w", err)
	}
	_, err := runRemote(ctx, c, name, token, []string{"grep", "-F", "-q", pathFile, bashrcFile})
	if err == nil {
		return nil
	}
	line := "\n. " + pathFile + "\n"
	if _, err := runRemoteStdin(ctx, c, name, token, []string{"tee", "-a", bashrcFile}, strings.NewReader(line)); err != nil {
		return fmt.Errorf("configure path: %w", err)
	}
	return nil
}

func ensureToolchain(ctx context.Context, c sdkClient, name, token string, kind project.Kind, arch *spriteArch, ready *bool) (string, bool, error) {
	probe, bin, err := toolPaths(kind)
	if err != nil {
		return "", false, err
	}
	if _, err := runRemote(ctx, c, name, token, probe); err == nil {
		return probe[0], false, nil
	}
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if !*ready {
		got, err := detectArch(ctx, c, name, token)
		if err != nil {
			return "", false, err
		}
		*arch = got
		*ready = true
	}
	if err := installToolchain(ctx, c, name, token, kind, *arch); err != nil {
		return "", false, err
	}
	return bin, true, nil
}

func toolPaths(kind project.Kind) (probe []string, bin string, err error) {
	switch kind {
	case project.KindGo:
		return []string{"go", "version"}, goBin, nil
	case project.KindRust:
		return []string{"cargo", "--version"}, cargoBin, nil
	case project.KindPython:
		return []string{"uv", "--version"}, uvBin, nil
	case project.KindNode:
		return []string{"npm", "--version"}, npmBin, nil
	default:
		return nil, "", fmt.Errorf("unknown project kind %q", kind)
	}
}

func detectArch(ctx context.Context, c sdkClient, name, token string) (spriteArch, error) {
	out, err := runRemote(ctx, c, name, token, []string{"uname", "-m"})
	if err != nil {
		return spriteArch{}, fmt.Errorf("detect sprite arch: %w", err)
	}
	arch, err := archFromUname(out)
	if err != nil {
		return spriteArch{}, fmt.Errorf("detect sprite arch: %w", err)
	}
	return arch, nil
}

func installToolchain(ctx context.Context, c sdkClient, name, token string, kind project.Kind, arch spriteArch) error {
	switch kind {
	case project.KindGo:
		return installGo(ctx, c, name, token, arch)
	case project.KindRust:
		return installRust(ctx, c, name, token, arch)
	case project.KindPython:
		return installUV(ctx, c, name, token, arch)
	case project.KindNode:
		return installNode(ctx, c, name, token, arch)
	default:
		return fmt.Errorf("unknown project kind %q", kind)
	}
}

func runProjectCommands(ctx context.Context, c sdkClient, name, token, workDir string, step project.Step, bin string, installed bool) error {
	var env []string
	if installed {
		env = toolEnv()
	}
	for _, argv := range step.Commands {
		if len(argv) == 0 {
			return fmt.Errorf("%s: empty command", step.Kind)
		}
		cmd := append([]string{}, argv...)
		cmd[0] = bin
		if _, err := runRemoteIn(ctx, c, name, token, workDir, env, cmd, nil); err != nil {
			return fmt.Errorf("%s: %w", step.Kind, err)
		}
	}
	return nil
}

func toolEnv() []string {
	return []string{
		"HOME=" + spriteHome,
		"USER=sprite",
		"PATH=" + localBin + ":" + localPrefix + "/go/bin:" + spriteHome + "/.cargo/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
	}
}

func installGo(ctx context.Context, c sdkClient, name, token string, arch spriteArch) error {
	asset := goAsset(arch)
	if err := downloadVerified(ctx, c, name, token, asset); err != nil {
		return installFailed("go", err)
	}
	if _, err := runRemote(ctx, c, name, token, []string{argvMkdir, "-p", localPrefix}); err != nil {
		return installFailed("go", err)
	}
	if _, err := runRemote(ctx, c, name, token, []string{argvTar, tarGzip, asset.archive, "-C", localPrefix}); err != nil {
		_ = removeRemote(ctx, c, name, token, asset.archive)
		return installFailed("go", err)
	}
	if err := removeRemote(ctx, c, name, token, asset.archive); err != nil {
		return installFailed("go", err)
	}
	return nil
}

func installNode(ctx context.Context, c sdkClient, name, token string, arch spriteArch) error {
	asset := nodeAsset(arch)
	if err := downloadVerified(ctx, c, name, token, asset); err != nil {
		return installFailed("node", err)
	}
	if err := extractStrip(ctx, c, name, token, asset.archive, nodePrefix); err != nil {
		return installFailed("node", err)
	}
	links := [][2]string{
		{nodePrefix + "/bin/node", localBin + "/node"},
		{nodePrefix + "/bin/npm", npmBin},
		{nodePrefix + "/bin/npx", localBin + "/npx"},
	}
	for _, link := range links {
		if _, err := runRemote(ctx, c, name, token, []string{"ln", "-sfn", link[0], link[1]}); err != nil {
			return installFailed("node", err)
		}
	}
	return nil
}

func installUV(ctx context.Context, c sdkClient, name, token string, arch spriteArch) error {
	asset := uvAsset(arch)
	if err := downloadVerified(ctx, c, name, token, asset); err != nil {
		return installFailed("uv", err)
	}
	const extractDir = "/tmp/sbox-uv"
	if err := extractStrip(ctx, c, name, token, asset.archive, extractDir); err != nil {
		return installFailed("uv", err)
	}
	if _, err := runRemote(ctx, c, name, token, []string{"mv", extractDir + "/uv", uvBin}); err != nil {
		return installFailed("uv", err)
	}
	if _, err := runRemote(ctx, c, name, token, []string{"mv", extractDir + "/uvx", localBin + "/uvx"}); err != nil {
		return installFailed("uv", err)
	}
	if _, err := runRemote(ctx, c, name, token, []string{"chmod", "755", uvBin, localBin + "/uvx"}); err != nil {
		return installFailed("uv", err)
	}
	if err := removeRemote(ctx, c, name, token, extractDir); err != nil {
		return installFailed("uv", err)
	}
	return nil
}

func installRust(ctx context.Context, c sdkClient, name, token string, arch spriteArch) error {
	asset := rustupAsset(arch)
	if err := downloadVerified(ctx, c, name, token, asset); err != nil {
		return installFailed("rust", err)
	}
	if _, err := runRemote(ctx, c, name, token, []string{"chmod", "755", asset.archive}); err != nil {
		return installFailed("rust", err)
	}
	argv := []string{asset.archive, "-y", "--no-modify-path", "--profile", "minimal", "--default-toolchain", "stable"}
	if _, err := runRemoteIn(ctx, c, name, token, "", toolEnv(), argv, nil); err != nil {
		_ = removeRemote(ctx, c, name, token, asset.archive)
		return installFailed("rust", err)
	}
	if err := removeRemote(ctx, c, name, token, asset.archive); err != nil {
		return installFailed("rust", err)
	}
	return nil
}

func downloadVerified(ctx context.Context, c sdkClient, name, token string, asset toolAsset) error {
	if _, err := runRemote(ctx, c, name, token, []string{"curl", "-fsSL", "-o", asset.archive, asset.url}); err != nil {
		return fmt.Errorf("download: %w", err)
	}
	sumOut, err := runRemote(ctx, c, name, token, []string{"sha256sum", asset.archive})
	if err != nil {
		_ = removeRemote(ctx, c, name, token, asset.archive)
		return fmt.Errorf("checksum: %w", err)
	}
	if !checksumMatches(sumOut, asset.sha256) {
		_ = removeRemote(ctx, c, name, token, asset.archive)
		return errors.New("checksum: archive hash does not match")
	}
	return nil
}

func extractStrip(ctx context.Context, c sdkClient, name, token, archive, dest string) error {
	if _, err := runRemote(ctx, c, name, token, []string{argvMkdir, "-p", dest}); err != nil {
		return err
	}
	if _, err := runRemote(ctx, c, name, token, []string{argvTar, tarGzip, archive, "-C", dest, "--strip-components=1"}); err != nil {
		_ = removeRemote(ctx, c, name, token, archive)
		return err
	}
	return removeRemote(ctx, c, name, token, archive)
}

func removeRemote(ctx context.Context, c sdkClient, name, token, path string) error {
	_, err := runRemote(ctx, c, name, token, []string{"rm", "-rf", path})
	return err
}

func installFailed(tool string, err error) error {
	return fmt.Errorf("install %s: %w", tool, err)
}
