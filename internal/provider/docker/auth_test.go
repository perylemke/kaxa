package docker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func jwtWithExp(exp time.Time) string {
	payload, _ := json.Marshal(map[string]int64{"exp": exp.Unix()})
	return "h." + base64.RawURLEncoding.EncodeToString(payload) + ".s"
}

func authServer(t *testing.T, token func(n int) string, status int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(calls.Add(1))
		var body authRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.Method != http.MethodPost || body.Identifier != "me" || body.Secret != testPAT {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		_, _ = fmt.Fprintf(w, `{"access_token":%q}`, token(n))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func newAuth(srv *httptest.Server, now func() time.Time) *authenticator {
	a := newAuthenticator("me", testPAT, srv.Client())
	a.url = srv.URL
	a.now = now
	return a
}

func TestTokenIsCachedUntilExpiry(t *testing.T) {
	t.Parallel()
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	srv, calls := authServer(t, func(n int) string {
		return jwtWithExp(clock.Add(10*time.Minute)) + strconv.Itoa(n)
	}, http.StatusOK)
	a := newAuth(srv, func() time.Time { return clock })
	first, err := a.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := a.Token(context.Background()); again != first || calls.Load() != 1 {
		t.Fatalf("token was exchanged %d times, want 1", calls.Load())
	}
	clock = clock.Add(9*time.Minute + 30*time.Second)
	if _, err := a.Token(context.Background()); err != nil || calls.Load() != 2 {
		t.Fatalf("expiring token must be exchanged again: calls = %d, err = %v", calls.Load(), err)
	}
}

func TestTokenWithoutExpiryLivesFiveMinutes(t *testing.T) {
	t.Parallel()
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	srv, calls := authServer(t, func(int) string { return "opaque" }, http.StatusOK)
	a := newAuth(srv, func() time.Time { return clock })
	_, _ = a.Token(context.Background())
	clock = clock.Add(3 * time.Minute)
	_, _ = a.Token(context.Background())
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
	clock = clock.Add(2 * time.Minute)
	_, _ = a.Token(context.Background())
	if calls.Load() != 2 {
		t.Fatalf("calls = %d, want 2", calls.Load())
	}
}

func TestTokenRejected(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status int
		code   string
	}{
		{http.StatusUnauthorized, codeUnauthenticated},
		{http.StatusForbidden, codePermissionDenied},
		{http.StatusInternalServerError, ""},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.status), func(t *testing.T) {
			t.Parallel()
			srv, _ := authServer(t, nil, tt.status)
			_, err := newAuth(srv, time.Now).Token(context.Background())
			apiErr, ok := errors.AsType[*APIError](err)
			if !ok || apiErr.Status != tt.status || apiErr.Code != tt.code {
				t.Fatalf("err = %v", err)
			}
			if strings.Contains(err.Error(), testPAT) {
				t.Fatal("the error holds the token")
			}
		})
	}
}

func TestTokenUnreadableResponse(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{"not json": "<html>", "no token": `{}`} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			defer srv.Close()
			if _, err := newAuth(srv, time.Now).Token(context.Background()); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestTokenAcceptsTokenField(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"token":"legacy"}`))
	}))
	defer srv.Close()
	got, err := newAuth(srv, time.Now).Token(context.Background())
	if err != nil || got != "legacy" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestAuthSecretsHoldPATAndToken(t *testing.T) {
	t.Parallel()
	srv, _ := authServer(t, func(int) string { return "tok-123" }, http.StatusOK)
	a := newAuth(srv, time.Now)
	_, _ = a.Token(context.Background())
	got := a.secrets()
	if len(got) != 2 || got[0] != testPAT || got[1] != "tok-123" {
		t.Fatalf("secrets = %v", got)
	}
}

func TestJWTExpiry(t *testing.T) {
	t.Parallel()
	want := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	if got, ok := jwtExpiry(jwtWithExp(want)); !ok || !got.Equal(want) {
		t.Fatalf("got %v, %v", got, ok)
	}
	for _, bad := range []string{"", "a.b", "a.!!!.c", "a." + base64.RawURLEncoding.EncodeToString([]byte("{}")) + ".c"} {
		if _, ok := jwtExpiry(bad); ok {
			t.Errorf("jwtExpiry(%q) must fail", bad)
		}
	}
}
