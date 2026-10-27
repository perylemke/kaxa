package sprites

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

const (
	apiServiceName   = "kaxa-api"
	apiStatusRunning = "running"
	apiStatusFailed  = "failed"
)

var (
	apiReadyTimeout  = 30 * time.Second
	apiReadyInterval = time.Second
)

// PublishAPI starts an HTTP service and routes the sprite URL to port.
func (p *Provider) PublishAPI(ctx context.Context, id sandbox.ID, workDir string, port int, argv []string) error {
	if err := validateName(string(id)); err != nil {
		return err
	}
	if err := validateAPIWorkDir(workDir); err != nil {
		return err
	}
	if port < 1 || port > 65535 {
		return usererr.New(fmt.Sprintf("invalid api port %d", port), "use a TCP port from 1 to 65535")
	}
	if err := validateAPIArgv(argv); err != nil {
		return err
	}
	c, token, err := p.clientAndToken()
	if err != nil {
		return err
	}
	if _, err := runRemote(ctx, c, string(id), token, apiServiceArgv(workDir, port, argv)); err != nil {
		return fmt.Errorf("start api service: %w", err)
	}
	return waitAPIReady(ctx, c, string(id), token)
}

// SetURLAuth sets the sprite URL authentication to public or sprite.
func (p *Provider) SetURLAuth(ctx context.Context, id sandbox.ID, auth string) error {
	if err := validateName(string(id)); err != nil {
		return err
	}
	if auth != sandbox.URLAuthPublic && auth != sandbox.URLAuthSprite {
		return usererr.New(
			fmt.Sprintf("invalid url auth %q", auth),
			"use public or sprite",
		)
	}
	c, token, err := p.clientAndToken()
	if err != nil {
		return err
	}
	client, ok := c.(*realSDKClient)
	if !ok {
		return usererr.New("url auth requires the sprites API", "pass --provider sprites")
	}
	body, err := json.Marshal(map[string]map[string]string{
		"url_settings": {"auth": auth},
	})
	if err != nil {
		return err
	}
	if err := putSpriteJSON(ctx, client.baseURL, token, "/v1/sprites/"+string(id), body); err != nil {
		return redactErr(err, token)
	}
	return nil
}

func apiServiceArgv(workDir string, port int, argv []string) []string {
	cmd := []string{spriteEnvCmd, servicesCmd, "create", apiServiceName, "--cmd", argv[0]}
	if len(argv) > 1 {
		cmd = append(cmd, "--args", strings.Join(argv[1:], ","))
	}
	return append(cmd,
		"--dir", workDir,
		"--http-port", strconv.Itoa(port),
		"--env", strings.Join(toolEnv(), ","),
		"--no-stream",
	)
}

func waitAPIReady(ctx context.Context, c sdkClient, name, token string) error {
	deadline := time.Now().Add(apiReadyTimeout)
	for {
		status, detail, err := apiServiceStatus(ctx, c, name, token)
		if err != nil {
			return err
		}
		switch status {
		case apiStatusRunning:
			return nil
		case apiStatusFailed:
			return apiServiceFailed(detail)
		}
		if !time.Now().Before(deadline) {
			return usererr.New(
				"api service did not become running",
				"check the command and the port",
			)
		}
		if err := sleepAPIReady(ctx); err != nil {
			return err
		}
	}
}

func apiServiceStatus(ctx context.Context, c sdkClient, name, token string) (string, string, error) {
	out, err := runRemote(ctx, c, name, token, []string{spriteEnvCmd, servicesCmd, "get", apiServiceName})
	if err != nil {
		return "", "", fmt.Errorf("read api service: %w", err)
	}
	status, detail := parseAPIState(out)
	return status, detail, nil
}

func parseAPIState(out string) (string, string) {
	trimmed := strings.TrimSpace(out)
	var body struct {
		State struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		} `json:"state"`
	}
	if trimmed == "" || json.Unmarshal([]byte(trimmed), &body) != nil {
		return "", ""
	}
	return body.State.Status, strings.TrimSpace(body.State.Error)
}

func apiServiceFailed(detail string) error {
	msg := "api service failed"
	if detail != "" {
		msg += ": " + detail
	}
	return usererr.New(msg, "check the command and the port")
}

func sleepAPIReady(ctx context.Context) error {
	timer := time.NewTimer(apiReadyInterval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func validateAPIWorkDir(dir string) error {
	if dir == spriteHome {
		return nil
	}
	prefix := workspaceDir + "/"
	rest, ok := strings.CutPrefix(dir, prefix)
	if !ok || rest == "" || strings.Contains(rest, "/") {
		return usererr.New(fmt.Sprintf("invalid api directory %q", dir), "use the sprite home or a workspace repository")
	}
	return validateRepoDir(rest)
}

func validateAPIArgv(argv []string) error {
	if len(argv) == 0 {
		return usererr.New("api command is required", "pass --api-cmd with comma-separated argv")
	}
	for _, arg := range argv {
		if arg == "" || strings.Contains(arg, ",") {
			return usererr.New("api command has an empty argument", "separate argv with commas and omit empty items")
		}
	}
	return nil
}

func putSpriteJSON(ctx context.Context, baseURL, token, path string, body []byte) (err error) {
	endpoint := strings.TrimRight(baseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}
