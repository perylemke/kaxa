package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

var dnsLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

type sandboxJSON struct {
	ID       string   `json:"id"`
	Provider string   `json:"provider"`
	Status   string   `json:"status"`
	Labels   []string `json:"labels,omitempty"`
}

type fieldJSON struct {
	Label string `json:"label"`
	Text  string `json:"text,omitempty"`
	At    string `json:"at,omitempty"`
}

type detailJSON struct {
	ID           string               `json:"id"`
	Provider     string               `json:"provider"`
	Status       string               `json:"status"`
	Labels       []string             `json:"labels,omitempty"`
	Fields       []fieldJSON          `json:"fields,omitempty"`
	URL          string               `json:"url,omitempty"`
	Auth         string               `json:"auth,omitempty"`
	CreatedAt    string               `json:"created_at,omitempty"`
	UpdatedAt    string               `json:"updated_at,omitempty"`
	Capabilities sandbox.Capabilities `json:"capabilities"`
	// ShellCommand opens the tmux shell of this sandbox from a local terminal.
	ShellCommand string `json:"shell_command"`
}

type okJSON struct {
	OK bool `json:"ok"`
}

// destroyJSON answers a destroy. Notice is set when the provider accepted the
// destroy but still lists the sandbox as stopped, as Blaxel does while it finishes.
type destroyJSON struct {
	OK     bool   `json:"ok"`
	Notice string `json:"notice,omitempty"`
}

func (s *Server) listSandboxes(w http.ResponseWriter, r *http.Request) {
	providers, names, err := s.sandboxTargets()
	if err != nil {
		s.writeErr(w, err)
		return
	}
	infos, failures, err := lifecycle.CollectSandboxes(r.Context(), providers, names)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	out := make([]sandboxJSON, 0, len(infos))
	for _, info := range infos {
		out = append(out, sandboxJSON{
			ID: string(info.ID), Provider: info.Provider, Status: info.Status, Labels: info.Labels,
		})
	}
	warnings := make([]string, 0, len(failures))
	for _, failure := range failures {
		warnings = append(warnings, s.redact(failure.Error()))
		s.event(levelWarn, "list", "", failure.Error())
	}
	s.writeJSON(w, http.StatusOK, struct {
		Sandboxes []sandboxJSON `json:"sandboxes"`
		Warnings  []string      `json:"warnings,omitempty"`
	}{Sandboxes: out, Warnings: warnings})
}

func (s *Server) sandboxTargets() (map[string]sandbox.Provider, []string, error) {
	if s.only != "" {
		p, err := s.openProvider(s.only)
		if err != nil {
			return nil, nil, err
		}
		return map[string]sandbox.Provider{p.Name(): p}, []string{p.Name()}, nil
	}
	return lifecycle.ListTargets(s.reg, s.cfg)
}

func (s *Server) inspectSandbox(w http.ResponseWriter, r *http.Request) {
	p, name, ok := s.sandboxFromPath(w, r)
	if !ok {
		return
	}
	detail, err := p.Inspect(r.Context(), sandbox.ID(name))
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, newDetail(p, detail))
}

func (s *Server) restartSandbox(w http.ResponseWriter, r *http.Request) {
	s.runAction(w, r, func(ctx context.Context, p sandbox.Provider, id sandbox.ID) error {
		restarter, ok := p.(sandbox.Restarter)
		if !ok {
			return lifecycle.Unsupported(p, sandbox.FeatureRestart)
		}
		return restarter.Restart(ctx, id)
	})
}

func (s *Server) upgradeSandbox(w http.ResponseWriter, r *http.Request) {
	s.runAction(w, r, func(ctx context.Context, p sandbox.Provider, id sandbox.ID) error {
		upgrader, ok := p.(sandbox.Upgrader)
		if !ok {
			return lifecycle.Unsupported(p, sandbox.FeatureUpgrade)
		}
		return upgrader.Upgrade(ctx, id)
	})
}

func (s *Server) runAction(
	w http.ResponseWriter,
	r *http.Request,
	run func(context.Context, sandbox.Provider, sandbox.ID) error,
) {
	p, name, ok := s.sandboxFromPath(w, r)
	if !ok {
		return
	}
	if err := run(r.Context(), p, sandbox.ID(name)); err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, okJSON{OK: true})
}

type urlAuthBody struct {
	Auth string `json:"auth"`
}

func (s *Server) urlAuth(w http.ResponseWriter, r *http.Request) {
	p, name, ok := s.sandboxFromPath(w, r)
	if !ok {
		return
	}
	var body urlAuthBody
	if err := decodeJSON(r, &body); err != nil {
		s.writeErr(w, err)
		return
	}
	if body.Auth != sandbox.URLAuthPublic && body.Auth != sandbox.URLAuthSprite {
		s.writeErr(w, usererr.New("invalid url auth "+body.Auth, "use public or sprite"))
		return
	}
	setter, allowed := p.(sandbox.PublicURL)
	if !allowed {
		s.writeErr(w, lifecycle.Unsupported(p, sandbox.FeatureURLAuth))
		return
	}
	id := sandbox.ID(name)
	if err := setter.SetURLAuth(r.Context(), id, body.Auth); err != nil {
		s.writeErr(w, err)
		return
	}
	detail, err := p.Inspect(r.Context(), id)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, newDetail(p, detail))
}

type destroyBody struct {
	Confirm string `json:"confirm"`
}

func (s *Server) destroySandbox(w http.ResponseWriter, r *http.Request) {
	p, name, ok := s.sandboxFromPath(w, r)
	if !ok {
		return
	}
	var body destroyBody
	if err := decodeJSON(r, &body); err != nil {
		s.writeErr(w, err)
		return
	}
	if body.Confirm != name {
		s.writeErr(w, usererr.New(
			"confirmation does not match "+name,
			"confirm the sandbox named on the page",
		))
		return
	}
	target := p.Name() + "/" + name
	s.event(levelInfo, "destroy", target, "started")
	s.event(levelInfo, "destroy", target, "confirmation matches "+name)
	s.destroyLookup(r.Context(), p, name, target)
	s.event(levelInfo, "destroy", target, "destroying on "+p.Name())
	if err := p.Destroy(r.Context(), sandbox.ID(name)); err != nil {
		s.writeErr(w, err)
		return
	}
	s.event(levelInfo, "destroy", target, "provider accepted the destroy")
	notice := s.destroyVerify(r.Context(), p, name, target)
	s.writeJSON(w, http.StatusOK, destroyJSON{OK: true, Notice: notice})
}

// destroyLookup logs what the sandbox is before it goes. A failed lookup does not
// stop the destroy, because a broken sandbox is often the one to remove.
func (s *Server) destroyLookup(ctx context.Context, p sandbox.Provider, name, target string) {
	s.event(levelInfo, "destroy", target, "looking up the sandbox")
	detail, err := p.Inspect(ctx, sandbox.ID(name))
	if err != nil {
		_, message, _ := s.describe(err)
		s.event(levelWarn, "destroy", target, "lookup failed, destroying anyway: "+message)
		return
	}
	s.event(levelInfo, "destroy", target, "found, status "+detail.Status)
}

// destroyVerify asks the provider again and logs whether the sandbox is gone.
// It returns a message for the page when the sandbox is still listed as stopped.
func (s *Server) destroyVerify(ctx context.Context, p sandbox.Provider, name, target string) string {
	s.event(levelInfo, "destroy", target, "checking that it is gone")
	infos, err := p.List(ctx)
	if err != nil {
		_, message, _ := s.describe(err)
		s.event(levelWarn, "destroy", target, "could not verify: "+message)
		return ""
	}
	for _, info := range infos {
		if string(info.ID) == name {
			message := "still listed with status " + info.Status + "; the provider may finish removing it"
			s.event(levelWarn, "destroy", target, message)
			if strings.EqualFold(info.Status, statusStopped) {
				return message
			}
			return ""
		}
	}
	s.event(levelInfo, "destroy", target, "gone from "+p.Name())
	return ""
}

func (s *Server) sandboxFromPath(w http.ResponseWriter, r *http.Request) (sandbox.Provider, string, bool) {
	provider := r.PathValue("provider")
	name := r.PathValue("name")
	if !dnsLabel.MatchString(provider) || !dnsLabel.MatchString(name) {
		s.writeErr(w, usererr.New("invalid sandbox path", "use the provider and sandbox name"))
		return nil, "", false
	}
	p, err := s.openProvider(provider)
	if err != nil {
		s.writeErr(w, err)
		return nil, "", false
	}
	return p, name, true
}

func (s *Server) openProvider(name string) (sandbox.Provider, error) {
	if s.only != "" && name != s.only {
		return nil, usererr.New(
			fmt.Sprintf("provider %q is not part of this session", name),
			"restart kaxa ui without --provider to see every provider",
		)
	}
	p, err := lifecycle.ResolveProvider(s.reg, s.cfg, name)
	if err != nil {
		if errors.Is(err, sandbox.ErrNotConfigured) {
			return nil, usererr.Wrap(err,
				fmt.Sprintf("the %s provider is not configured", name),
				"set its KAXA_* credentials and run kaxa ui again",
			)
		}
		return nil, usererr.Wrap(err, err.Error(), "check the provider name and its credentials")
	}
	if !sandbox.IsAvailable(p) {
		return nil, usererr.New(
			fmt.Sprintf("the %s provider is not available yet", p.Name()),
			"choose a provider that is available",
		)
	}
	return p, nil
}

func newDetail(p sandbox.Provider, detail sandbox.Detail) detailJSON {
	fields := make([]fieldJSON, 0, len(detail.Fields))
	for _, field := range detail.Fields {
		if field.Text == "" && field.At.IsZero() {
			continue
		}
		item := fieldJSON{Label: field.Label, Text: field.Text}
		if !field.At.IsZero() {
			item.At = field.At.UTC().Format(time.RFC3339)
		}
		fields = append(fields, item)
	}
	out := detailJSON{
		ID:           string(detail.ID),
		Provider:     detail.Provider,
		Status:       detail.Status,
		Labels:       detail.Labels,
		Fields:       fields,
		URL:          detail.URL,
		Auth:         detail.Auth,
		Capabilities: sandbox.CapabilitiesOf(p),
	}
	if out.Provider == "" {
		out.Provider = p.Name()
	}
	out.ShellCommand = shellCommand(out.Provider, out.ID)
	if !detail.CreatedAt.IsZero() {
		out.CreatedAt = detail.CreatedAt.UTC().Format(time.RFC3339)
	}
	if !detail.UpdatedAt.IsZero() {
		out.UpdatedAt = detail.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return out
}

func validateSandboxName(name string) error {
	if !dnsLabel.MatchString(name) {
		return usererr.New(
			fmt.Sprintf("invalid sandbox name %q", name),
			"use a DNS label: lowercase letters, digits, and hyphens",
		)
	}
	return nil
}
