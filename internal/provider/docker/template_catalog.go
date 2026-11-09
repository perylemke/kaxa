package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
)

const (
	hubTagsHost  = "hub.docker.com"
	hubTagsPath  = "/v2/repositories/docker/sandbox-templates/tags"
	hubPageSize  = 100
	hubMaxPages  = 10
	hubTimeout   = 15 * time.Second
	templateRepo = "docker/sandbox-templates"
)

// tagLister returns the raw tag names of docker/sandbox-templates.
type tagLister interface {
	ListTags(ctx context.Context) ([]string, error)
}

// hubTagName is a single Docker Hub tag. Slashes and dot-dot stay out of the image reference.
var hubTagName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

// versionInTag matches a pinned release such as 0.11.0 or claude-code-0.11.0.
var versionInTag = regexp.MustCompile(`(?:^|-)\d+\.\d+`)

type hubTagPage struct {
	Next    *string `json:"next"`
	Results []struct {
		Name string `json:"name"`
	} `json:"results"`
}

// hubClient reads public tags from Docker Hub. It does not send the Docker PAT.
type hubClient struct {
	// base overrides the Hub URL in tests. Empty uses hub.docker.com.
	base string
	http *http.Client
}

func newHubClient() *hubClient {
	return &hubClient{http: hubHTTPClient()}
}

func hubHTTPClient() *http.Client {
	return &http.Client{
		Timeout: hubTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// ListTags pages the public tag list. The page number is ours; the next URL
// from Hub is only a signal to stop, never a request target.
func (c *hubClient) ListTags(ctx context.Context) ([]string, error) {
	var tags []string
	for page := 1; page <= hubMaxPages; page++ {
		body, err := c.fetchPage(ctx, page)
		if err != nil {
			return nil, err
		}
		if len(body.Results) == 0 {
			return tags, nil
		}
		for _, row := range body.Results {
			tags = append(tags, row.Name)
		}
		if body.Next == nil || *body.Next == "" {
			return tags, nil
		}
	}
	return tags, nil
}

func (c *hubClient) fetchPage(ctx context.Context, page int) (hubTagPage, error) {
	endpoint, err := c.pageURL(page)
	if err != nil {
		return hubTagPage{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return hubTagPage{}, fmt.Errorf("docker: hub request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return hubTagPage{}, fmt.Errorf("docker: hub tags: %w", urlErrorCause(err))
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return hubTagPage{}, fmt.Errorf("docker: read hub tags: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return hubTagPage{}, fmt.Errorf("docker: hub returned status %d", resp.StatusCode)
	}
	var body hubTagPage
	if err := json.Unmarshal(raw, &body); err != nil {
		return hubTagPage{}, fmt.Errorf("docker: decode hub tags: %w", err)
	}
	return body, nil
}

func (c *hubClient) pageURL(page int) (string, error) {
	u := url.URL{Scheme: "https", Host: hubTagsHost, Path: hubTagsPath}
	if c.base != "" {
		parsed, err := url.Parse(c.base)
		if err != nil {
			return "", fmt.Errorf("docker: hub url: %w", err)
		}
		u = *parsed
	}
	q := u.Query()
	q.Set("page_size", strconv.Itoa(hubPageSize))
	q.Set("page", strconv.Itoa(page))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// ListTemplates returns the floating tags of docker/sandbox-templates, sorted
// by tag. Nightly, latest, and pinned versions are left out. The option name
// is the image reference passed to create.
func (p *Provider) ListTemplates(ctx context.Context) ([]sandbox.TemplateOption, error) {
	if _, err := p.ready(); err != nil {
		return nil, err
	}
	tags, err := p.tagSource().ListTags(ctx)
	if err != nil {
		return nil, err
	}
	return templateOptions(tags), nil
}

func (p *Provider) tagSource() tagLister {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.tags != nil {
		return p.tags
	}
	return newHubClient()
}

func templateOptions(tags []string) []sandbox.TemplateOption {
	out := make([]sandbox.TemplateOption, 0, len(tags))
	for _, tag := range tags {
		if !floatingTag(tag) {
			continue
		}
		out = append(out, sandbox.TemplateOption{
			Name:  templateRepo + ":" + tag,
			Label: tag,
		})
	}
	slices.SortFunc(out, func(a, b sandbox.TemplateOption) int {
		return strings.Compare(a.Label, b.Label)
	})
	return out
}

func floatingTag(name string) bool {
	if !hubTagName.MatchString(name) {
		return false
	}
	if name == "latest" || strings.Contains(name, "nightly") {
		return false
	}
	return !versionInTag.MatchString(name)
}
