package docker

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/perylemke/kaxa/internal/sandbox"
)

const restoredRes = "sandboxes/new"

func newIndexForTest(t *testing.T) *restoredIndex {
	t.Helper()
	return newRestoredIndex(filepath.Join(t.TempDir(), "kaxa", restoredFile))
}

func TestRestoredIndexStartsEmpty(t *testing.T) {
	t.Parallel()
	got, err := newIndexForTest(t).labels()
	if err != nil || len(got) != 0 {
		t.Fatalf("labels = %v, err = %v", got, err)
	}
}

func TestRestoredIndexAddsReplacesAndRemoves(t *testing.T) {
	t.Parallel()
	x := newIndexForTest(t)
	for _, e := range []restoredEntry{
		{Resource: restoredRes, Name: "a", Labels: []string{sandbox.SyncLabel}},
		{Resource: "sandboxes/other", Name: "b"},
		{Resource: restoredRes, Name: "a", Labels: []string{sandbox.SyncLabel, labelKaxa}},
	} {
		if err := x.add(e); err != nil {
			t.Fatal(err)
		}
	}
	got, err := x.labels()
	if err != nil || len(got) != 2 || !slices.Equal(got[restoredRes], []string{sandbox.SyncLabel, labelKaxa}) {
		t.Fatalf("labels = %v, err = %v; an entry of the same resource must be replaced", got, err)
	}
	if err := x.remove(restoredRes); err != nil {
		t.Fatal(err)
	}
	if err := x.remove("sandboxes/unknown"); err != nil {
		t.Fatalf("removing an unknown sandbox must not fail: %v", err)
	}
	if got, _ := x.labels(); len(got) != 1 {
		t.Fatalf("labels = %v, want only the other sandbox", got)
	}
}

func TestRestoredIndexIsPrivate(t *testing.T) {
	t.Parallel()
	x := newIndexForTest(t)
	if err := x.add(restoredEntry{Resource: restoredRes, Name: "a"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(x.path)
	if err != nil || info.Mode().Perm() != restoredFileMode {
		t.Fatalf("file mode = %v, err = %v, want %v", info.Mode().Perm(), err, os.FileMode(restoredFileMode))
	}
}

func TestRestoredIndexRejectsBadFiles(t *testing.T) {
	t.Parallel()
	for name, content := range map[string]string{
		"not json":    "{",
		"new version": `{"version":99,"sandboxes":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			x := newIndexForTest(t)
			if err := os.MkdirAll(filepath.Dir(x.path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(x.path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := x.labels(); err == nil {
				t.Fatal("a file kaxa cannot read must be an error, not an empty index")
			}
			if err := x.add(restoredEntry{Resource: restoredRes}); err == nil {
				t.Fatal("add must not overwrite a file it cannot read")
			}
		})
	}
}

const listWithRestored = `{"sandboxes":[` +
	`{"name":"sandboxes/new","displayName":"copy","core":{"status":"running"}},` +
	`{"name":"sandboxes/theirs","displayName":"theirs","core":{"status":"running"}},` +
	`{"name":"sandboxes/mine","displayName":"mine","core":{"status":"running","labels":{"` + managedLabel + `":"true"}}}]}`

func TestClientAdoptsRestoredSandboxes(t *testing.T) {
	t.Parallel()
	c, _ := newAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/sandboxes" {
			_, _ = w.Write([]byte(listWithRestored))
			return
		}
		_, _ = w.Write([]byte(`{"name":"sandboxes/new","displayName":"copy","core":{"status":"running"}}`))
	})
	c.restored = func() (map[string][]string, error) {
		return map[string][]string{restoredRes: {sandbox.SyncLabel}}, nil
	}

	all, err := c.List(context.Background())
	if err != nil || len(all) != 2 {
		t.Fatalf("list = %+v, err = %v, want the restored sandbox and the labeled one", all, err)
	}
	var restored Record
	for _, r := range all {
		if r.Name == "copy" {
			restored = r
		}
	}
	if !restored.Managed() || !slices.Equal(restored.Labels, []string{sandbox.SyncLabel}) {
		t.Fatalf("restored = %+v, want kaxa's, with the labels of the source", restored)
	}
	if _, err := c.Find(context.Background(), "theirs"); err == nil {
		t.Fatal("a sandbox of the user must stay out of reach")
	}
	got, err := c.Get(context.Background(), restoredRes)
	if err != nil || !got.Managed() || !slices.Equal(got.Labels, []string{sandbox.SyncLabel}) {
		t.Fatalf("get = %+v, err = %v", got, err)
	}
}

func TestClientListFailsWhenTheIndexCannotBeRead(t *testing.T) {
	t.Parallel()
	c, _ := newAPIServer(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(listWithRestored)) })
	c.restored = func() (map[string][]string, error) { return nil, os.ErrPermission }
	if _, err := c.List(context.Background()); err == nil {
		t.Fatal("an unreadable index must be an error, because sandboxes would silently vanish")
	}
}
