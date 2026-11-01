package docker

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultAuthURL = "https://hub.docker.com/v2/auth/token"

	// fallbackTokenLife applies when the access token is not a JWT with an exp
	// claim. Refreshing early only costs one more request.
	fallbackTokenLife = 5 * time.Minute
	tokenSkew         = time.Minute
	maxAuthBody       = 64 << 10
)

// authenticator exchanges a Docker ID and a personal access token for the
// short-lived access token the Sandboxes API accepts as a bearer token, and
// keeps it until shortly before it expires.
type authenticator struct {
	url  string
	http *http.Client
	now  func() time.Time

	mu      sync.Mutex
	id      string
	pat     string
	token   string
	expires time.Time
}

func newAuthenticator(id, pat string, hc *http.Client) *authenticator {
	return &authenticator{url: defaultAuthURL, http: hc, now: time.Now, id: id, pat: pat}
}

// Token returns a valid access token, exchanging the personal access token
// when there is none or it is about to expire.
func (a *authenticator) Token(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.token != "" && a.now().Add(tokenSkew).Before(a.expires) {
		return a.token, nil
	}
	token, err := a.exchange(ctx)
	if err != nil {
		return "", err
	}
	a.token, a.expires = token, a.now().Add(fallbackTokenLife)
	if exp, ok := jwtExpiry(token); ok {
		a.expires = exp
	}
	return a.token, nil
}

// secrets lists everything that must never appear in a message.
func (a *authenticator) secrets() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return []string{a.pat, a.token}
}

type authRequest struct {
	Identifier string `json:"identifier"`
	Secret     string `json:"secret"`
}

type authResponse struct {
	AccessToken string `json:"access_token"`
	Token       string `json:"token"`
}

func (a *authenticator) exchange(ctx context.Context) (string, error) {
	body, err := json.Marshal(authRequest{Identifier: a.id, Secret: a.pat}) //gosec:disable G117 -- the sign-in body carries the token to Docker, by design
	if err != nil {
		return "", fmt.Errorf("docker: encode sign-in: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("docker: build sign-in: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("docker: sign in to Docker: %w", urlErrorCause(err))
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxAuthBody))
	if err != nil {
		return "", fmt.Errorf("docker: read sign-in response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", &APIError{Status: resp.StatusCode, Code: codeForStatus(resp.StatusCode)}
	}
	var out authResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", errors.New("docker: Docker sign-in returned an unreadable response")
	}
	token := out.AccessToken
	if token == "" {
		token = out.Token
	}
	if token == "" {
		return "", errors.New("docker: Docker sign-in returned no access token")
	}
	return token, nil
}

// codeForStatus names the API code for the statuses sign-in answers with.
func codeForStatus(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return codeUnauthenticated
	case http.StatusForbidden:
		return codePermissionDenied
	}
	return ""
}

// jwtExpiry reads the exp claim of a JWT without verifying it: the value only
// decides when to ask for a new token.
func jwtExpiry(token string) (time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, false
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == 0 {
		return time.Time{}, false
	}
	return time.Unix(claims.Exp, 0), true
}
