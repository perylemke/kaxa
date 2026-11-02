package blaxel

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	sdk "github.com/blaxel-ai/sdk-go"
)

func TestRecordFromKeepsOnlyManaged(t *testing.T) {
	t.Parallel()
	if _, ok := recordFrom(&sdk.Sandbox{Metadata: sdk.Metadata{Name: "foreign"}}); ok {
		t.Fatal("unmanaged sandbox was kept")
	}
	if _, ok := recordFrom(nil); ok {
		t.Fatal("nil sandbox was kept")
	}
	rec, ok := recordFrom(&sdk.Sandbox{
		Metadata: sdk.Metadata{
			Name: boxName, CreatedAt: "2026-01-02T03:04:05Z",
			Labels: map[string]string{
				labelManaged: labelTrue, labelName: boxName, labelLabels: "sync,team",
				labelAPIPort: "8000", labelTemplate: defaultImage,
			},
		},
		Spec:   sdk.SandboxSpec{Region: defaultRegion, Runtime: sdk.SandboxRuntime{Image: defaultImage}},
		Status: "DEPLOYED",
	})
	if !ok || rec.Name != boxName || rec.Status != statusRunning || rec.APIPort != 8000 || rec.Image != defaultImage {
		t.Fatalf("record = %+v, ok = %v", rec, ok)
	}
	if len(rec.Labels) != 2 || rec.Labels[0] != "sync" || rec.Labels[1] != "team" {
		t.Fatalf("labels = %v", rec.Labels)
	}
	if rec.CreatedAt != time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) {
		t.Fatalf("created = %s", rec.CreatedAt)
	}
}

func TestKaxaLabels(t *testing.T) {
	t.Parallel()
	labels := kaxaLabels(CreateParams{Name: boxName, Image: defaultImage, Labels: []string{"sync"}, APIPort: 8000})
	if labels[labelManaged] != labelTrue || labels[labelName] != boxName || labels[labelLabels] != "sync" || labels[labelAPIPort] != "8000" {
		t.Fatalf("labels = %v", labels)
	}
	without := kaxaLabels(CreateParams{Name: boxName, Image: defaultImage})
	if _, ok := without[labelAPIPort]; ok {
		t.Fatal("port label set without a port")
	}
}

func TestMapStatus(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"DEPLOYED": statusRunning, "FAILED": statusError, "ARCHIVED": statusArchived,
		"ARCHIVING": statusArchived, "DEACTIVATED": statusStopped, "TERMINATED": statusStopped,
		"DEPLOYING": statusStarting, "": statusStarting,
	}
	for in, want := range cases {
		if got := mapStatus(in); got != want {
			t.Errorf("mapStatus(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSandboxEndpoint(t *testing.T) {
	t.Parallel()
	got, err := sandboxEndpoint("https://sbx-box-kaxa.us-pdx-1.bl.run", "process/kaxa-1/stdin")
	if err != nil || got != "https://sbx-box-kaxa.us-pdx-1.bl.run/process/kaxa-1/stdin" {
		t.Fatalf("endpoint = %q, %v", got, err)
	}
	for _, bad := range []string{
		"http://sbx.bl.run", "https://user:pw@sbx.bl.run", "https://example.com", "://nope", "",
	} {
		if _, err := sandboxEndpoint(bad, "process/x/stdin"); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestConvertErr(t *testing.T) {
	t.Parallel()
	if !errors.Is(convertErr(&sdk.Error{StatusCode: http.StatusNotFound}), ErrNotFound) {
		t.Fatal("404 did not become not found")
	}
	if !errors.Is(convertErr(&sdk.Error{StatusCode: http.StatusConflict}), ErrAlreadyExists) {
		t.Fatal("409 was not already exists")
	}
	err := convertErr(&sdk.Error{StatusCode: http.StatusUnauthorized, Message: "nope"})
	api, ok := errors.AsType[*APIError](err)
	if !ok || api.Status != http.StatusUnauthorized || api.Message != "nope" {
		t.Fatalf("err = %v", err)
	}
	if convertErr(nil) != nil {
		t.Fatal("nil error changed")
	}
}

func TestUserErrHidesSecret(t *testing.T) {
	t.Parallel()
	err := userErr(&APIError{Status: http.StatusUnauthorized, Message: "bad test-key"}, boxName, "test-key")
	if err == nil || strings.Contains(err.Error(), "test-key") {
		t.Fatalf("err = %v", err)
	}
	plain := userErr(errors.New("dial failed with test-key"), boxName, "test-key")
	if plain == nil || strings.Contains(plain.Error(), "test-key") || !strings.Contains(plain.Error(), "<redacted>") {
		t.Fatalf("plain = %v", plain)
	}
}
