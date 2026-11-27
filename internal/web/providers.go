package web

import (
	"net/http"

	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

type providerInfo struct {
	Name         string               `json:"name"`
	Available    bool                 `json:"available"`
	Default      bool                 `json:"default"`
	Capabilities sandbox.Capabilities `json:"capabilities"`
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
		out = append(out, providerInfo{
			Name:         name,
			Available:    sandbox.IsAvailable(p),
			Default:      name == s.cfg.Provider,
			Capabilities: sandbox.CapabilitiesOf(p),
		})
	}
	return out, nil
}
