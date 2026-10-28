package modal_test

import (
	"context"
	"slices"
	"testing"

	"github.com/perylemke/kaxa/internal/sandbox"
)

const (
	testLognameEnv = "LOGNAME=modal"
	testEnvBinary  = "env"
)

func TestRepoExistsRunsTestDirAsModal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		code int
		want bool
	}{
		{"directory present", 0, true},
		{"directory absent", 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := syncedFake(t)
			f.process = newFakeProcess("", "", tt.code)
			p := newProvider(t, f)
			var _ sandbox.RepoChecker = p

			exists, err := p.RepoExists(context.Background(), testName, "api")
			if err != nil || exists != tt.want {
				t.Fatalf("RepoExists = %v, %v, want %v", exists, err, tt.want)
			}
			want := []string{
				testRunuser, "-u", testModalUser, "--", testEnvBinary, testHomeEnv, testUserEnv, testLognameEnv,
				"test", "-d", testWorkspace + "/api",
			}
			if !slices.Equal(f.execArgv, want) {
				t.Fatalf("argv = %q, want %q", f.execArgv, want)
			}
		})
	}
}

func TestRepoExistsReportsRealFailures(t *testing.T) {
	t.Parallel()

	f := syncedFake(t)
	f.process = newFakeProcess("", "", 2)

	exists, err := newProvider(t, f).RepoExists(context.Background(), testName, "api")
	if err == nil || exists {
		t.Fatalf("RepoExists = %v, %v, want an error", exists, err)
	}
}

func TestRepoExistsRejectsBadInput(t *testing.T) {
	t.Parallel()

	f := syncedFake(t)
	p := newProvider(t, f)
	if _, err := p.RepoExists(context.Background(), "Bad Name", "api"); err == nil {
		t.Fatal("invalid sandbox name accepted")
	}
	for _, repo := range []string{"", ".", "..", "a/b", `a\b`} {
		if _, err := p.RepoExists(context.Background(), testName, repo); err == nil {
			t.Fatalf("repo %q accepted", repo)
		}
	}
	if f.execArgv != nil {
		t.Fatalf("a command ran for invalid input: %v", f.execArgv)
	}
}
