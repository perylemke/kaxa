package web

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

const (
	defaultAPIPort = 8000
	contentNDJSON  = "application/x-ndjson"
)

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

// createEvent is one line of the create response. A response is zero or more
// steps, then exactly one of done or error.
type createEvent struct {
	Step  string     `json:"step,omitempty"`
	Done  *createOut `json:"done,omitempty"`
	Error string     `json:"error,omitempty"`
	Hint  string     `json:"hint,omitempty"`
	// ID is set on an error when the sandbox was kept.
	ID string `json:"id,omitempty"`
}

// createSandbox validates the request before it answers. A refused request is a
// plain JSON error. An accepted one is a stream of createEvent lines, so the page
// can show each step of a create that takes minutes.
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
	noteTarget(w, body.Provider+"/"+body.Name)
	p, err := s.openProvider(body.Provider)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	req, err := s.buildCreateRequest(body)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	lifecycle.NormalizeRepoList(&req)
	if err := lifecycle.Check(p, req); err != nil {
		s.writeErr(w, err)
		return
	}
	s.streamCreate(w, r, p, req)
}

func (s *Server) streamCreate(w http.ResponseWriter, r *http.Request, p sandbox.Provider, req lifecycle.Request) {
	target := p.Name() + "/" + req.Name
	w.Header().Set("Content-Type", contentNDJSON)
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	flusher := http.NewResponseController(w)
	send := func(ev createEvent) {
		_ = enc.Encode(ev)
		_ = flusher.Flush()
	}
	started := s.clock()
	s.event(levelInfo, "create", target, "started")
	run := func(ctx context.Context, label string, task func(context.Context) error) error {
		s.event(levelInfo, "create", target, label)
		send(createEvent{Step: s.redact(label)})
		return task(ctx)
	}
	res, err := lifecycle.Create(r.Context(), p, req, run)
	if err != nil {
		_, message, hint := s.describe(err)
		s.event(levelError, "create", target, message)
		send(createEvent{Error: message, Hint: hint, ID: string(res.ID)})
		return
	}
	out := s.created(r, p, res, target)
	s.event(levelInfo, "create", target, "done in "+elapsed(s.clock().Sub(started)))
	send(createEvent{Done: &out})
}

func (s *Server) created(r *http.Request, p sandbox.Provider, res lifecycle.Result, target string) createOut {
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
		s.event(levelWarn, "create", target, "inspect: "+err.Error())
		return out
	}
	out.URL = detail.URL
	out.Auth = detail.Auth
	return out
}

func (s *Server) buildCreateRequest(body createBody) (lifecycle.Request, error) {
	req := lifecycle.Request{
		Name:    body.Name,
		Running: body.Running,
		Sync:    body.Sync,
		Public:  body.Public,
	}
	if err := s.applySync(&req, body); err != nil {
		return lifecycle.Request{}, err
	}
	if err := applyAPI(&req, body); err != nil {
		return lifecycle.Request{}, err
	}
	return req, nil
}

func (s *Server) applySync(req *lifecycle.Request, body createBody) error {
	if !body.Sync {
		if body.Dir != "" {
			return usererr.New("dir requires sync", "turn on sync to copy that repository")
		}
		if body.NoInstall {
			return usererr.New("no_install requires sync", "turn on sync to copy the repository without installing its dependencies")
		}
		return nil
	}
	if err := s.checkDirs(body.Dir); err != nil {
		return err
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
