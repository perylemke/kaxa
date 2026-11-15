package web

import (
	"context"
	"net/http"
	"os"
	"strings"

	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

const defaultAPIPort = 8000

type createBody struct {
	Name      string   `json:"name"`
	Provider  string   `json:"provider"`
	Running   bool     `json:"running"`
	Sync      bool     `json:"sync"`
	NoInstall bool     `json:"no_install"`
	Dir       string   `json:"dir"`
	API       bool     `json:"api"`
	APICmd    []string `json:"api_cmd"`
	APIPort   int      `json:"api_port"`
	APIEnv    []string `json:"api_env"`
	Public    bool     `json:"public"`
}

type createOut struct {
	ID              string     `json:"id"`
	Provider        string     `json:"provider"`
	WorkDir         string     `json:"work_dir,omitempty"`
	SkippedCommands [][]string `json:"skipped_commands,omitempty"`
	URL             string     `json:"url,omitempty"`
	Auth            string     `json:"auth,omitempty"`
	PreviewToken    string     `json:"preview_token,omitempty"`
}

func (s *Server) createSandbox(w http.ResponseWriter, r *http.Request) {
	var body createBody
	if err := decodeJSON(r, &body); err != nil {
		s.writeErr(w, err)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.Provider = strings.TrimSpace(body.Provider)
	body.Dir = strings.TrimSpace(body.Dir)
	if err := validateSandboxName(body.Name); err != nil {
		s.writeErr(w, err)
		return
	}
	p, err := s.openProvider(body.Provider)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	req, err := buildCreateRequest(body)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	res, err := lifecycle.Create(r.Context(), p, req, s.progress)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, s.created(r, p, res))
}

func (s *Server) progress(ctx context.Context, label string, task func(context.Context) error) error {
	s.log(label)
	return task(ctx)
}

func (s *Server) created(r *http.Request, p sandbox.Provider, res lifecycle.Result) createOut {
	out := createOut{
		ID:              string(res.ID),
		Provider:        p.Name(),
		WorkDir:         res.WorkDir,
		SkippedCommands: res.Skipped,
	}
	if cred, ok := p.(sandbox.PreviewCredential); ok {
		out.PreviewToken = cred.TakePreviewToken(res.ID)
	}
	detail, err := p.Inspect(r.Context(), res.ID)
	if err != nil {
		s.log(err.Error())
		return out
	}
	out.URL = detail.URL
	out.Auth = detail.Auth
	return out
}

func buildCreateRequest(body createBody) (lifecycle.Request, error) {
	req := lifecycle.Request{
		Name:    body.Name,
		Running: body.Running,
		Sync:    body.Sync,
		Public:  body.Public,
	}
	if err := applySync(&req, body); err != nil {
		return lifecycle.Request{}, err
	}
	if err := applyAPI(&req, body); err != nil {
		return lifecycle.Request{}, err
	}
	return req, nil
}

func applySync(req *lifecycle.Request, body createBody) error {
	if !body.Sync {
		if body.Dir != "" {
			return usererr.New("dir requires sync", "turn on sync to copy that repository")
		}
		if body.NoInstall {
			return usererr.New("no_install requires sync", "turn on sync to copy the repository without installing its dependencies")
		}
		return nil
	}
	copies, err := lifecycle.ResolveRepos(body.Dir, body.Dir != "")
	if err != nil {
		return err
	}
	req.NoInstall = body.NoInstall
	req.Repos = copies
	if len(copies) > 0 {
		req.Root = copies[0].Root
		req.Repo = copies[0].Repo
	}
	return nil
}

func applyAPI(req *lifecycle.Request, body createBody) error {
	if !body.API {
		if len(body.APICmd) > 0 {
			return usererr.New("api command requires api", "turn on api to publish the service")
		}
		if body.APIPort != 0 {
			return usererr.New("api port requires api", "turn on api to publish the service")
		}
		if len(body.APIEnv) > 0 {
			return usererr.New("api env requires api", "turn on api to publish the service")
		}
		return nil
	}
	port := body.APIPort
	if port == 0 {
		port = defaultAPIPort
	}
	if err := lifecycle.ValidateAPI(body.Running, port, body.APICmd); err != nil {
		return err
	}
	env, err := lifecycle.ResolveAPIEnv(body.APIEnv, os.LookupEnv)
	if err != nil {
		return err
	}
	req.API = true
	req.APIPort = port
	req.APICmd = body.APICmd
	req.APIEnv = env
	return nil
}
