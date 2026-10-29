package reposync

import (
	"errors"
	"strings"
	"testing"
)

const bareFalse = "bare = false"

func TestIsSecretFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		rel  string
		want bool
	}{
		{".env", true},
		{"app/.env", true},
		{".env.local", true},
		{".env.production", true},
		{".env.example", false},
		{".env.sample", false},
		{"server.pem", true},
		{"certs/tls.KEY", true},
		{"store.p12", true},
		{"id_rsa", true},
		{"home/.ssh/id_ed25519", true},
		{"id_rsa.pub", false},
		{".netrc", true},
		{".npmrc", true},
		{".pypirc", true},
		{"credentials.json", true},
		{"credentials-prod.json", true},
		{"main.go", false},
		{"README.md", false},
		{"keyboard.go", false},
		{"environment.md", false},
	}
	for _, tt := range tests {
		t.Run(tt.rel, func(t *testing.T) {
			t.Parallel()
			if got := IsSecretFile(tt.rel); got != tt.want {
				t.Fatalf("IsSecretFile(%q) = %v, want %v", tt.rel, got, tt.want)
			}
		})
	}
}

func TestSanitizeGitConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    []string
		missing []string
	}{
		{ //gosec:disable G101 -- fixture: a fake token inside a remote URL
			name:    "token in https remote",
			in:      "[remote \"origin\"]\n\turl = https://x-access-token:ghp_secret@github.com/org/repo.git\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n",
			want:    []string{"url = https://github.com/org/repo.git", "fetch = +refs/heads/*"},
			missing: []string{"ghp_secret", "x-access-token"},
		},
		{
			name:    "token as username",
			in:      "[remote \"origin\"]\n\turl = https://ghp_secret@github.com/org/repo.git\n",
			want:    []string{"url = https://github.com/org/repo.git"},
			missing: []string{"ghp_secret"},
		},
		{
			name: "ssh remotes are kept",
			in:   "[remote \"a\"]\n\turl = git@github.com:org/repo.git\n[remote \"b\"]\n\turl = ssh://git@github.com/org/repo.git\n",
			want: []string{"url = git@github.com:org/repo.git", "url = ssh://git@github.com/org/repo.git"},
		},
		{
			name:    "extraheader",
			in:      "[http \"https://github.com/\"]\n\textraheader = AUTHORIZATION: basic c2VjcmV0\n[core]\n\tbare = false\n",
			want:    []string{"[http \"https://github.com/\"]", bareFalse},
			missing: []string{"c2VjcmV0", "extraheader"},
		},
		{
			name:    "extraheader on the header line",
			in:      "[http] extraHeader = Authorization: bearer abc123\n[core]\n\tbare = false\n",
			want:    []string{"[http]", bareFalse},
			missing: []string{"abc123"},
		},
		{
			name:    "credential section",
			in:      "[credential]\n\thelper = store --file=/home/u/.secret\n[core]\n\tbare = false\n",
			want:    []string{"[core]", bareFalse},
			missing: []string{"credential", "helper", ".secret"},
		},
		{
			name:    "credential section scoped to a host",
			in:      "[credential \"https://github.com\"]\n\tusername = me\n\thelper = osxkeychain\n[user]\n\tname = me\n",
			want:    []string{"[user]", "name = me"},
			missing: []string{"osxkeychain", "credential"},
		},
		{
			name:    "continued extraheader value",
			in:      "[http]\n\textraheader = Authorization: \\\n  bearer abc123\n[core]\n\tbare = false\n",
			want:    []string{"[core]", bareFalse},
			missing: []string{"abc123"},
		},
		{
			name:    "url rewrite section",
			in:      "[url \"https://tok123@github.com/\"]\n\tinsteadOf = https://github.com/\n",
			want:    []string{"[url \"https://github.com/\"]", "insteadOf = https://github.com/"},
			missing: []string{"tok123"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := string(sanitizeGitConfig([]byte(tt.in)))
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Fatalf("missing %q in:\n%s", w, got)
				}
			}
			for _, m := range tt.missing {
				if strings.Contains(strings.ToLower(got), strings.ToLower(m)) {
					t.Fatalf("kept %q in:\n%s", m, got)
				}
			}
		})
	}
}

func TestArchiveStripsGitCredentials(t *testing.T) {
	root := initRepo(t)
	isolateGitConfig(t)
	writeFile(t, root, "main.go", "package main\n")
	git(t, root, "add", "main.go")
	git(t, root, "remote", "add", "origin", "https://x-access-token:ghp_secret@github.com/org/repo.git")
	git(t, root, "config", "http.https://github.com/.extraheader", "AUTHORIZATION: basic c2VjcmV0")
	writeFile(t, root, ".git/credentials", "https://user:pw@github.com\n")

	rc, err := Archive(t.Context(), root, "api")
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	t.Cleanup(func() { _ = rc.Close() })
	entries := readTar(t, rc)

	cfg, ok := entries["api/.git/config"]
	if !ok {
		t.Fatal("missing .git/config")
	}
	for _, leak := range []string{"ghp_secret", "x-access-token", "c2VjcmV0"} {
		if strings.Contains(cfg.Body, leak) {
			t.Fatalf("config leaks %q:\n%s", leak, cfg.Body)
		}
	}
	if !strings.Contains(cfg.Body, "https://github.com/org/repo.git") {
		t.Fatalf("config lost the remote:\n%s", cfg.Body)
	}
	if _, ok := entries["api/.git/credentials"]; ok {
		t.Fatal(".git/credentials was archived")
	}
}

func TestArchiveRefusesUntrackedSecrets(t *testing.T) {
	root := initRepo(t)
	isolateGitConfig(t)
	writeFile(t, root, "main.go", "package main\n")
	git(t, root, "add", "main.go")
	writeFile(t, root, ".env.example", "KEY=\n")
	writeFile(t, root, "config/credentials.json", "{}")

	_, err := Archive(t.Context(), root, "api")
	if !errors.Is(err, ErrSecretFile) || !strings.Contains(err.Error(), "config/credentials.json") {
		t.Fatalf("err = %v", err)
	}
}

func TestArchiveAllowsIgnoredAndTrackedSecretNames(t *testing.T) {
	root := initRepo(t)
	isolateGitConfig(t)
	writeFile(t, root, ".gitignore", ".env\n")
	writeFile(t, root, ".env", "TOKEN=abc\n")
	writeFile(t, root, ".env.example", "TOKEN=\n")
	writeFile(t, root, "fixtures/test.pem", "public test fixture\n")
	git(t, root, "add", ".gitignore", ".env.example", "fixtures/test.pem")

	rc, err := Archive(t.Context(), root, "api")
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	t.Cleanup(func() { _ = rc.Close() })
	entries := readTar(t, rc)
	if _, ok := entries["api/.env"]; ok {
		t.Fatal("ignored .env was archived")
	}
	assertRegular(t, entries, "api/.env.example", "TOKEN=\n")
	assertRegular(t, entries, "api/fixtures/test.pem", "")
}
