package sandbox_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/sandbox"
)

const (
	logLevelInfo = "LOG_LEVEL=info"
	errReserved  = "reserved"
)

func TestParseEnvFlag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		raw    string
		want   []string
		errHas string
	}{
		{name: "empty"},
		{name: "one", raw: logLevelInfo, want: []string{logLevelInfo}},
		{name: "two", raw: "DATABASE_URL=postgres://localhost/app," + logLevelInfo, want: []string{"DATABASE_URL=postgres://localhost/app", logLevelInfo}},
		{name: "space after comma", raw: "A=1, B=2", want: []string{"A=1", "B=2"}},
		{name: "empty value", raw: "EMPTY=", want: []string{"EMPTY="}},
		{name: "keeps equals", raw: "Q=a=b", want: []string{"Q=a=b"}},
		{name: "last wins", raw: "A=1,A=2", want: []string{"A=2"}},
		{name: "bare key", raw: "DATABASE_URL", errHas: "DATABASE_URL needs a value"},
		{name: "comma splits a value", raw: "A=hello,world", errHas: "needs a value"},
		{name: "control character", raw: "A=b\nc", errHas: "control character"},
		{name: "reserved path", raw: "PATH=/bin", errHas: errReserved},
		{name: "reserved home", raw: "HOME=/", errHas: errReserved},
		{name: "kaxa prefix", raw: "KAXA_SPRITES_TOKEN=x", errHas: errReserved},
		{name: "modal prefix", raw: "MODAL_TOKEN_SECRET=x", errHas: errReserved},
		{name: "fly prefix", raw: "FLY_API_TOKEN=x", errHas: errReserved},
		{name: "sprites prefix", raw: "SPRITES_TOKEN=x", errHas: errReserved},
		{name: "bad key", raw: "1BAD=x", errHas: "invalid env key"},
		{name: "empty entry", raw: "A=1,", errHas: "empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := sandbox.ParseEnvFlag(tt.raw)
			if tt.errHas != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errHas) {
					t.Fatalf("err = %v, want %q", err, tt.errHas)
				}
				if strings.Contains(err.Error(), "postgres://") || strings.Contains(err.Error(), "secret") {
					t.Fatalf("error leaks a value: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseEnvFile(t *testing.T) {
	t.Parallel()

	body := `
# comment
export DATABASE_URL="postgres://localhost/app"

LOG_LEVEL='info'
EMPTY=
Q=a=b
KEEP=$HOME
TICK=say` + "`id`" + `
A=1
A=2
`
	got, err := sandbox.ParseEnvFile(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := []string{
		"DATABASE_URL=postgres://localhost/app",
		logLevelInfo,
		"EMPTY=",
		"Q=a=b",
		"KEEP=$HOME",
		"TICK=say`id`",
		"A=2",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestParseEnvFileRejectsReservedAndComma(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		body   string
		errHas string
	}{
		{name: "reserved", body: "PATH=/bin\n", errHas: errReserved},
		{name: "comma", body: "A=1,2\n", errHas: "comma"},
		{name: "control", body: "A=b\tc\n", errHas: "control character"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := sandbox.ParseEnvFile(strings.NewReader(tt.body))
			if err == nil || !strings.Contains(err.Error(), tt.errHas) {
				t.Fatalf("err = %v, want %q", err, tt.errHas)
			}
			if strings.Contains(err.Error(), "/bin") || strings.Contains(err.Error(), "1,2") {
				t.Fatalf("error leaks a value: %v", err)
			}
		})
	}
}

func TestReadEnvFileRejectsMissingDirectoryAndSymlink(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	_, err := sandbox.ReadEnvFile(filepath.Join(dir, "missing.env"))
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing: %v", err)
	}
	if _, err := sandbox.ReadEnvFile(dir); err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("directory: %v", err)
	}
	target := filepath.Join(dir, "real.env")
	if err := os.WriteFile(target, []byte("A=1\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	link := filepath.Join(dir, "link.env")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if _, err := sandbox.ReadEnvFile(link); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink: %v", err)
	}
	got, err := sandbox.ReadEnvFile(target)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !slices.Equal(got, []string{"A=1"}) {
		t.Fatalf("got %v", got)
	}
}

func TestResolveSandboxEnvFlagOverridesFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("A=from-file\nB=2\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := sandbox.ResolveSandboxEnv(path, "A=from-flag,C=3")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	want := []string{"A=from-flag", "B=2", "C=3"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMergeEnvLaterLayerWins(t *testing.T) {
	t.Parallel()

	got := sandbox.MergeEnv(
		[]string{"HOME=/home/user", "PATH=/usr/bin"},
		[]string{"MODE=sandbox", "PATH=/tmp"},
		[]string{"MODE=api"},
	)
	want := []string{"HOME=/home/user", "PATH=/tmp", "MODE=api"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if sandbox.MergeEnv(nil, nil) != nil {
		t.Fatal("empty merge should be nil")
	}
}
