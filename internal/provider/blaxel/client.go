package blaxel

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	sdk "github.com/blaxel-ai/sdk-go"
	"github.com/blaxel-ai/sdk-go/option"
)

// identPattern keeps a process id safe to put in a URL path.
var identPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,80}$`)

// sdkClient is the apiClient and runtime over the official Blaxel Go SDK.
// Credentials are request options, never read from BL_API_KEY or ~/.blaxel.
type sdkClient struct {
	apiKey    string
	workspace string
	sandboxes *sdk.SandboxService
	http      *http.Client
	// termDial replaces the terminal WebSocket dial in tests.
	termDial wsDialFunc
}

func newSDKClient(apiKey, workspace string) *sdkClient {
	opts := []option.RequestOption{
		option.WithBaseURL(controlPlaneURL),
		option.WithAPIKey(apiKey),
		option.WithWorkspace(workspace),
	}
	svc := sdk.NewSandboxService(opts...)
	return &sdkClient{
		apiKey: apiKey, workspace: workspace, sandboxes: &svc,
		http: &http.Client{CheckRedirect: refuseRedirect},
	}
}

func refuseRedirect(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

func (s *sdkClient) Create(ctx context.Context, p CreateParams) (Record, error) {
	body := sdk.SandboxNewParams{Sandbox: sdk.SandboxParam{
		Metadata: sdk.MetadataParam{Name: p.Name, Labels: kaxaLabels(p)},
		Spec: sdk.SandboxSpecParam{
			Region: sdk.String(p.Region),
			Runtime: sdk.SandboxRuntimeParam{
				Image: sdk.String(p.Image), Memory: sdk.Int(int64(p.Memory)), Ports: portParams(p.APIPort),
			},
		},
	}}
	inst, err := s.sandboxes.NewInstance(ctx, body)
	if err != nil {
		return Record{}, convertErr(err)
	}
	if inst == nil || inst.Sandbox == nil {
		return Record{}, errors.New("blaxel: create returned no sandbox")
	}
	rec, ok := recordFrom(inst.Sandbox)
	if !ok {
		return Record{}, errors.New("blaxel: create returned an unmanaged sandbox")
	}
	return rec, nil
}

func (s *sdkClient) List(ctx context.Context) ([]Record, error) {
	page, err := s.sandboxes.ListInstances(ctx, sdk.SandboxListParams{})
	if err != nil {
		return nil, convertErr(err)
	}
	var out []Record
	for page != nil {
		out = append(out, managedRecords(page.Data)...)
		page, err = page.NextPage(ctx)
		if err != nil {
			return nil, convertErr(err)
		}
	}
	return out, nil
}

func (s *sdkClient) Find(ctx context.Context, name string) (Record, error) {
	inst, err := s.get(ctx, name)
	if err != nil {
		return Record{}, err
	}
	rec, ok := recordFrom(inst.Sandbox)
	if !ok {
		return Record{}, ErrNotFound
	}
	return rec, nil
}

func (s *sdkClient) Delete(ctx context.Context, name string) error {
	_, err := s.sandboxes.Delete(ctx, name)
	return convertErr(err)
}

func (s *sdkClient) PreviewURL(ctx context.Context, name string) (string, error) {
	inst, err := s.get(ctx, name)
	if err != nil {
		return "", err
	}
	preview, err := inst.Previews.Get(ctx, previewName)
	if err != nil {
		return "", convertErr(err)
	}
	return previewURL(preview)
}

func (s *sdkClient) CreatePreview(ctx context.Context, name string, port int) (string, error) {
	inst, err := s.get(ctx, name)
	if err != nil {
		return "", err
	}
	preview, err := inst.Previews.NewIfNotExists(ctx, sdk.SandboxPreviewNewParams{
		Preview: sdk.PreviewParam{
			Metadata: sdk.PreviewMetadataParam{Name: previewName},
			Spec:     sdk.PreviewSpecParam{Port: sdk.Int(int64(port)), Public: sdk.Bool(true)},
		},
	})
	if err != nil {
		return "", convertErr(err)
	}
	return previewURL(preview)
}

func (s *sdkClient) Run(ctx context.Context, name string, spec runSpec) (int, error) {
	inst, ident, err := s.start(ctx, name, spec, false)
	if err != nil {
		return 0, err
	}
	stream := inst.Process.StreamLogs(ctx, ident, sdk.ProcessStreamOptions{
		OnStdout: func(chunk string) { writeOut(spec.stdout, chunk) },
		OnStderr: func(chunk string) { writeOut(spec.stderr, chunk) },
	})
	if stream != nil && stream.Close != nil {
		defer stream.Close()
	}
	if spec.stdin != nil {
		if err := s.writeStdin(ctx, inst, ident, spec.stdin); err != nil {
			s.kill(inst, ident)
			return 0, err
		}
	}
	result, err := inst.Process.Wait(ctx, ident, -1, time.Second)
	if err != nil {
		if ctx.Err() != nil {
			s.kill(inst, ident)
		}
		return 0, convertErr(err)
	}
	if result == nil {
		return 0, errors.New("blaxel: process returned no result")
	}
	return int(result.ExitCode), nil
}

func (s *sdkClient) Start(ctx context.Context, name string, spec runSpec) error {
	_, _, err := s.start(ctx, name, spec, true)
	return err
}

func (s *sdkClient) start(ctx context.Context, name string, spec runSpec, waitPort bool) (*sdk.SandboxInstance, string, error) {
	inst, err := s.instance(ctx, name)
	if err != nil {
		return nil, "", err
	}
	ident, err := procName()
	if err != nil {
		return nil, "", err
	}
	res, err := inst.Process.New(ctx, processBody(spec, ident, waitPort))
	if err != nil {
		return nil, "", convertErr(err)
	}
	ident = processID(res, ident)
	if waitPort && res != nil && serviceExited(res.Status) {
		return nil, "", apiServiceFailed(int(res.ExitCode), trimOutput(res.Stderr))
	}
	return inst, ident, nil
}

func (s *sdkClient) Upload(ctx context.Context, name, path string, data []byte) error {
	inst, err := s.instance(ctx, name)
	if err != nil {
		return err
	}
	_, err = inst.FS.WriteBinary(ctx, path, data, "0644")
	return convertErr(err)
}

func (s *sdkClient) Remove(ctx context.Context, name, path string) error {
	inst, err := s.instance(ctx, name)
	if err != nil {
		return err
	}
	_, err = inst.FS.RM(ctx, path, false)
	return convertErr(err)
}

func (s *sdkClient) get(ctx context.Context, name string) (*sdk.SandboxInstance, error) {
	inst, err := s.sandboxes.GetInstance(ctx, name)
	if err != nil {
		return nil, convertErr(err)
	}
	if inst == nil || inst.Sandbox == nil {
		return nil, ErrNotFound
	}
	return inst, nil
}

func (s *sdkClient) instance(ctx context.Context, name string) (*sdk.SandboxInstance, error) {
	inst, err := s.get(ctx, name)
	if err != nil {
		return nil, err
	}
	if inst.Metadata.URL == "" {
		return nil, errors.New("blaxel: sandbox has no URL")
	}
	return inst, nil
}

func (s *sdkClient) writeStdin(ctx context.Context, inst *sdk.SandboxInstance, id string, body io.Reader) error {
	if err := s.stdinRequest(ctx, http.MethodPost, inst.Metadata.URL, id, body); err != nil {
		return err
	}
	return s.stdinRequest(ctx, http.MethodDelete, inst.Metadata.URL, id, nil)
}

func (s *sdkClient) stdinRequest(ctx context.Context, method, base, id string, body io.Reader) error {
	if !identPattern.MatchString(id) {
		return errors.New("blaxel: invalid process id")
	}
	endpoint, err := sandboxEndpoint(base, "process/"+id+"/stdin")
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("X-Blaxel-Authorization", "Bearer "+s.apiKey)
	req.Header.Set("X-Blaxel-Workspace", s.workspace)
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	resp, err := s.http.Do(req) //gosec:disable G107,G704 -- host is restricted to a Blaxel sandbox URL
	if err != nil {
		return s.redact(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, failureOutput))
	return s.redact(&APIError{Status: resp.StatusCode, Message: strings.TrimSpace(string(msg))})
}

func (s *sdkClient) kill(inst *sdk.SandboxInstance, id string) {
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	_, _ = inst.Process.Kill(ctx, id)
}

func (s *sdkClient) redact(err error) error {
	if err == nil || s.apiKey == "" {
		return err
	}
	msg := strings.ReplaceAll(err.Error(), s.apiKey, "<redacted>")
	msg = strings.ReplaceAll(msg, url.QueryEscape(s.apiKey), "<redacted>")
	if msg == err.Error() {
		return err
	}
	return errors.New(msg)
}

func processBody(spec runSpec, name string, waitPort bool) sdk.ProcessRequestParam {
	body := sdk.ProcessRequestParam{
		Command: quoteArgv(spec.argv), Name: sdk.String(name),
		KeepAlive: sdk.Bool(true), Timeout: sdk.Int(0), Env: envMap(spec.env),
	}
	if spec.dir != "" {
		body.WorkingDir = sdk.String(spec.dir)
	}
	if spec.stdin != nil {
		body.Stdin = sdk.Bool(true)
	}
	if waitPort && spec.port > 0 {
		body.WaitForPorts = []int64{int64(spec.port)}
	}
	return body
}

func kaxaLabels(p CreateParams) map[string]string {
	labels := map[string]string{
		labelManaged: labelTrue, labelName: p.Name,
		labelLabels: strings.Join(p.Labels, ","), labelTemplate: p.Image,
	}
	if p.APIPort > 0 {
		labels[labelAPIPort] = strconv.Itoa(p.APIPort)
	}
	return labels
}

func portParams(port int) []sdk.PortParam {
	if port == 0 {
		return nil
	}
	return []sdk.PortParam{{
		Target: int64(port), Name: sdk.String(previewName), Protocol: sdk.PortProtocolHTTP,
	}}
}

func managedRecords(instances []*sdk.SandboxInstance) []Record {
	out := make([]Record, 0, len(instances))
	for _, inst := range instances {
		if inst == nil {
			continue
		}
		if rec, ok := recordFrom(inst.Sandbox); ok {
			out = append(out, rec)
		}
	}
	return out
}

func recordFrom(sb *sdk.Sandbox) (Record, bool) {
	if sb == nil || sb.Metadata.Labels[labelManaged] != labelTrue {
		return Record{}, false
	}
	rec := Record{
		Name: sb.Metadata.Name, Status: mapStatus(string(sb.Status)),
		Labels: splitLabels(sb.Metadata.Labels[labelLabels]),
		Image:  sb.Spec.Runtime.Image, Region: sb.Spec.Region,
		CreatedAt: parseTime(sb.Metadata.CreatedAt),
	}
	if raw := sb.Metadata.Labels[labelAPIPort]; raw != "" {
		if port, err := strconv.Atoi(raw); err == nil {
			rec.APIPort = port
		}
	}
	return rec, true
}

func mapStatus(deployment string) string {
	switch deployment {
	case "DEPLOYED":
		return statusRunning
	case "FAILED":
		return statusError
	case "ARCHIVED", "ARCHIVING":
		return statusArchived
	case "DEACTIVATED", "DEACTIVATING", "DELETING", "TERMINATED":
		return statusStopped
	default:
		return statusStarting
	}
}

func splitLabels(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseTime(raw string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func previewURL(preview *sdk.PreviewInstance) (string, error) {
	if preview == nil || preview.Preview == nil || preview.Spec.URL == "" {
		return "", errors.New("blaxel: preview has no URL")
	}
	return preview.Spec.URL, nil
}

func procName() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("blaxel: process name: %w", err)
	}
	return "kaxa-" + hex.EncodeToString(b[:]), nil
}

func processID(res *sdk.ProcessResponse, fallback string) string {
	if res != nil && res.Name != "" {
		return res.Name
	}
	if res != nil && res.Pid != "" {
		return res.Pid
	}
	return fallback
}

func serviceExited(status sdk.ProcessResponseStatus) bool {
	switch status {
	case sdk.ProcessResponseStatusRunning:
		return false
	case sdk.ProcessResponseStatusFailed, sdk.ProcessResponseStatusKilled,
		sdk.ProcessResponseStatusStopped, sdk.ProcessResponseStatusCompleted:
		return true
	default:
		return true
	}
}

func envMap(entries []string) map[string]string {
	if len(entries) == 0 {
		return nil
	}
	out := make(map[string]string, len(entries))
	for _, entry := range entries {
		key, val, ok := strings.Cut(entry, "=")
		if ok {
			out[key] = val
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func writeOut(w io.Writer, chunk string) {
	if w == nil || chunk == "" {
		return
	}
	_, _ = io.WriteString(w, chunk)
}

func trimOutput(raw string) string {
	if len(raw) <= failureOutput {
		return raw
	}
	return raw[:failureOutput]
}

func sandboxEndpoint(base, path string) (string, error) {
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Host == "" {
		return "", errors.New("blaxel: sandbox URL is not https")
	}
	if !strings.HasSuffix(u.Hostname(), ".bl.run") {
		return "", errors.New("blaxel: sandbox URL is not a Blaxel host")
	}
	u.RawQuery, u.Fragment = "", ""
	u.Path = strings.TrimRight(u.Path, "/") + "/" + path
	return u.String(), nil
}

func convertErr(err error) error {
	if err == nil {
		return nil
	}
	var api *sdk.Error
	if !errors.As(err, &api) {
		return err
	}
	switch api.StatusCode {
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusConflict:
		return ErrAlreadyExists
	default:
		return &APIError{Status: api.StatusCode, Message: api.Message}
	}
}
