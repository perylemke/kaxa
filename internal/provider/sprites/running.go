package sprites

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	runningTaskPayload = `{"name":"sbox","expire":"1h"}`
	webtmuxCommit      = "0612eeb71406c83603044fcb8e351a12e864afd5"
	webtmuxDir         = "/home/sprite/.local/bin"
	webtmuxBin         = webtmuxDir + "/webtmux"
	webtmuxArchive     = "/tmp/sbox-webtmux.tar.gz"
)

// RunningHoldArgv is the remote command used to register a 1-hour task hold.
var RunningHoldArgv = []string{
	"sprite-env", "curl", "-fsS",
	"-X", "POST",
	"-H", "Content-Type: application/json",
	"-d", runningTaskPayload,
	"/v1/tasks",
}

// RunningHTTPServiceArgv starts webtmux on port 8080 attached to a tmux session named main.
var RunningHTTPServiceArgv = []string{
	"sprite-env", "services", "create", "webtmux",
	"--cmd", webtmuxBin,
	"--args", "-w,--no-auth,tmux,new-session,-A,-s,main",
	"--http-port", "8080",
	"--no-stream",
}

type webtmuxAsset struct {
	name   string
	sha256 string
}

var webtmuxAssets = map[string]webtmuxAsset{
	"x86_64": {
		name:   "webtmux-linux-amd64",
		sha256: "bb69984aab06d9463ed5da46a81b71d2d1582ba75c9c07c65f49fecbb52f7b7b",
	},
	"aarch64": {
		name:   "webtmux-linux-arm64",
		sha256: "b041b1dfb67027cbb17a93febd7389a277d5a7635b8a3539bace707bb34848d2",
	},
	"arm64": {
		name:   "webtmux-linux-arm64",
		sha256: "b041b1dfb67027cbb17a93febd7389a277d5a7635b8a3539bace707bb34848d2",
	},
}

func registerRunningHold(ctx context.Context, c sdkClient, name, token string) error {
	if _, err := runRemote(ctx, c, name, token, RunningHoldArgv); err != nil {
		return fmt.Errorf("register running hold: %w", err)
	}
	return nil
}

func registerRunningHTTPService(ctx context.Context, c sdkClient, name, token string) error {
	if err := installWebtmux(ctx, c, name, token); err != nil {
		return err
	}
	if _, err := runRemote(ctx, c, name, token, RunningHTTPServiceArgv); err != nil {
		return fmt.Errorf("start tmux terminal: %w", err)
	}
	return nil
}

func installWebtmux(ctx context.Context, c sdkClient, name, token string) error {
	archOut, err := runRemote(ctx, c, name, token, []string{"uname", "-m"})
	if err != nil {
		return fmt.Errorf("detect sprite arch: %w", err)
	}
	asset, ok := webtmuxAssets[strings.TrimSpace(archOut)]
	if !ok {
		return fmt.Errorf("detect sprite arch: unsupported %q", strings.TrimSpace(archOut))
	}

	if _, err := runRemote(ctx, c, name, token, []string{"mkdir", "-p", webtmuxDir}); err != nil {
		return fmt.Errorf("install webtmux: %w", err)
	}
	url := fmt.Sprintf("https://raw.githubusercontent.com/superfly/webtmux-sprites-demo/%s/builds/dist/%s.tar.gz", webtmuxCommit, asset.name)
	if _, err := runRemote(ctx, c, name, token, []string{"curl", "-fsSL", "-o", webtmuxArchive, url}); err != nil {
		return fmt.Errorf("download webtmux: %w", err)
	}
	sumOut, err := runRemote(ctx, c, name, token, []string{"sha256sum", webtmuxArchive})
	if err != nil {
		_ = removeWebtmuxArchive(ctx, c, name, token)
		return fmt.Errorf("checksum webtmux: %w", err)
	}
	if !checksumMatches(sumOut, asset.sha256) {
		_ = removeWebtmuxArchive(ctx, c, name, token)
		return errors.New("checksum webtmux: archive hash does not match")
	}
	if _, err := runRemote(ctx, c, name, token, []string{"tar", "-xzf", webtmuxArchive, "-C", "/tmp"}); err != nil {
		_ = removeWebtmuxArchive(ctx, c, name, token)
		return fmt.Errorf("extract webtmux: %w", err)
	}
	if _, err := runRemote(ctx, c, name, token, []string{"mv", "/tmp/" + asset.name, webtmuxBin}); err != nil {
		return fmt.Errorf("install webtmux: %w", err)
	}
	if _, err := runRemote(ctx, c, name, token, []string{"chmod", "755", webtmuxBin}); err != nil {
		return fmt.Errorf("install webtmux: %w", err)
	}
	return removeWebtmuxArchive(ctx, c, name, token)
}

func removeWebtmuxArchive(ctx context.Context, c sdkClient, name, token string) error {
	_, err := runRemote(ctx, c, name, token, []string{"rm", "-f", webtmuxArchive})
	return err
}

func configureRunning(ctx context.Context, c sdkClient, name, token string) error {
	if err := registerRunningHold(ctx, c, name, token); err != nil {
		return err
	}
	return registerRunningHTTPService(ctx, c, name, token)
}

func runRemote(ctx context.Context, c sdkClient, name, token string, argv []string) (string, error) {
	var stdout, stderr bytes.Buffer
	if err := c.Run(ctx, name, argv, &stdout, &stderr); err != nil {
		msg := mapExecErr(err, token).Error()
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			return "", fmt.Errorf("%s: %s", msg, detail)
		}
		return "", fmt.Errorf("%s", msg)
	}
	return stdout.String(), nil
}

func checksumMatches(output, want string) bool {
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
