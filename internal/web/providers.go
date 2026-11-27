package web

import (
	"context"
	"net/http"
	"time"

	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

type providerInfo struct {
	Name         string               `json:"name"`
	Available    bool                 `json:"available"`
	Default      bool                 `json:"default"`
	Capabilities sandbox.Capabilities `json:"capabilities"`
	// Compute lists the CPU and memory choices, for a provider that publishes them.
	Compute *sandbox.ComputeOptions `json:"compute,omitempty"`
}

func (s *Server) listProviders(w http.ResponseWriter, _ *http.Request) {
	infos, err := s.providerInfos()
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, struct {
		Providers []providerInfo `json:"providers"`
	}{Providers: infos})
}

func (s *Server) providerInfos() ([]providerInfo, error) {
	providers := s.reg.All()
	if s.only != "" {
		p, err := s.reg.Get(s.only)
		if err != nil {
			return nil, usererr.New("unknown provider "+s.only, "choose a provider from kaxa ui")
		}
		providers = []sandbox.Provider{p}
	}
	out := make([]providerInfo, 0, len(providers))
	for _, p := range providers {
		name := p.Name()
		_, _ = lifecycle.ResolveProvider(s.reg, s.cfg, name)
		info := providerInfo{
			Name:         name,
			Available:    sandbox.IsAvailable(p),
			Default:      name == s.cfg.Provider,
			Capabilities: sandbox.CapabilitiesOf(p),
		}
		if options, ok := sandbox.ComputeOptionsOf(p); ok {
			info.Compute = &options
		}
		out = append(out, info)
	}
	return out, nil
}

// templateTTL bounds a catalog request, which goes over the network.
const templateTTL = 10 * time.Second

type templateJSON struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

// listTemplates answers with the ready-made images of one provider. A provider
// without a catalog, or whose catalog fails, answers an empty list, and the page
// keeps its free-text field.
func (s *Server) listTemplates(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !dnsLabel.MatchString(name) {
		s.writeErr(w, usererr.New("invalid provider name", "use the provider name"))
		return
	}
	p, err := s.openProvider(name)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	out := struct {
		Templates []templateJSON `json:"templates"`
		Warning   string         `json:"warning,omitempty"`
	}{Templates: []templateJSON{}}
	if catalog, ok := p.(sandbox.TemplateCatalog); ok {
		ctx, cancel := context.WithTimeout(r.Context(), templateTTL)
		defer cancel()
		options, listErr := catalog.ListTemplates(ctx)
		if listErr != nil {
			_, message, _ := s.describe(listErr)
			s.event(levelWarn, "templates", name, message)
			out.Warning = message
		}
		for _, option := range options {
			out.Templates = append(out.Templates, templateJSON{Name: option.Name, Label: option.Label})
		}
	}
	s.writeJSON(w, http.StatusOK, out)
}
