package daytona

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/usererr"
)

func TestAPIErrorMessage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		err  APIError
		want string
	}{
		{APIError{Status: 500}, "daytona: API returned status 500"},
		{APIError{Status: 400, Message: "bad"}, "daytona: API returned status 400: bad"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			expect(t, msgKey, tt.err.Error(), tt.want)
		})
	}
}

func TestUserErrExplainsAPIStatuses(t *testing.T) {
	t.Parallel()
	const secret = "s3cret-key"
	tests := []struct {
		name   string
		status int
		wantUE bool
	}{
		{"unauthorized", http.StatusUnauthorized, true},
		{"forbidden", http.StatusForbidden, true},
		{"rate limited", http.StatusTooManyRequests, true},
		{"other", http.StatusBadGateway, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := userErr(&APIError{Status: tt.status, Message: "token " + secret}, boxName, secret)
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("secret leaked: %v", err)
			}
			if _, ok := errors.AsType[*usererr.Error](err); ok != tt.wantUE {
				t.Fatalf("user error = %v, want %v (%v)", ok, tt.wantUE, err)
			}
		})
	}
	if userErr(nil, boxName) != nil {
		t.Error("nil error changed")
	}
	for _, e := range []error{ErrAlreadyExists, ErrNotFound} {
		if _, ok := errors.AsType[*usererr.Error](userErr(e, boxName)); !ok {
			t.Errorf("%v is not a user error", e)
		}
	}
}

func TestEnrichRecordFillsEveryField(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t)
	f.override = func(r *http.Request) (int, any, bool) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/sandbox/"+sandboxID {
			return 0, nil, false
		}
		body := f.sandboxJSON()
		maps.Copy(body, map[string]any{
			"snapshot": "snap", "public": true, "spot": true, "gpu": 1, "gpuType": "H100",
			"errorReason": "oops", "desiredState": stateStarted, "sandboxClass": "linux-vm",
			"volumes": []any{
				map[string]any{"volumeId": "v1", "mountPath": "/data"},
				map[string]any{"volumeId": "vol-2", "mountPath": ""},
				map[string]any{"volumeId": "", "mountPath": ""},
			},
			"autoStopInterval": 5, "autoPauseInterval": 6, "autoArchiveInterval": 7, "autoDeleteInterval": 8,
			"autoDestroyAt": "soon", "lastActivityAt": "now", "updatedAt": "later",
			"kvm": true, "networkBlockAll": true, "networkAllowList": "1.1.1.1", "domainAllowList": "a.com",
		})
		return http.StatusOK, body, true
	}
	rec, err := f.client().Get(context.Background(), sandboxID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	enrichRecord(nil, nil)
	expect(t, "volumes", rec.VolumeMounts, []string{"/data", "vol-2"})
	expect(t, "gpu type", rec.GpuType, "H100")
	expect(t, "desired", rec.DesiredState, stateStarted)
	expect(t, "class", rec.SandboxClass, "linux-vm")
	expect(t, "allow", rec.DomainAllowList, "a.com")
	expect(t, "destroy", rec.AutoDestroyAt, "soon")
	expect(t, "kvm", rec.Kvm, true)

	rec.DesiredState = statusStopped
	rec.RawState = stateStarted
	fields := daytonaDetailFields(rec, "https://u", "public")
	labels := map[string]bool{}
	for _, f := range fields {
		labels[f.Label] = true
	}
	for _, want := range []string{"GPU", "Desired state", "Auto stop", "Volume", "URL", "Auth"} {
		if !labels[want] {
			t.Errorf("missing field %q in %v", want, fields)
		}
	}
}
