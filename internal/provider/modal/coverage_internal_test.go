package modal

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	sdk "github.com/modal-labs/modal-client/go"

	"github.com/perylemke/kaxa/internal/sandbox"
)

// stubProcess is a scripted process for runQuiet.
type stubProcess struct {
	stdin   *nopWriteCloser
	stdout  io.Reader
	stderr  io.Reader
	code    int
	waitErr error
}

type nopWriteCloser struct {
	w        io.Writer
	closeErr error
}

func (n *nopWriteCloser) Write(b []byte) (int, error) { return n.w.Write(b) }
func (n *nopWriteCloser) Close() error                { return n.closeErr }

func (s *stubProcess) Stdin() io.WriteCloser { return s.stdin }
func (s *stubProcess) Stdout() io.Reader     { return s.stdout }
func (s *stubProcess) Stderr() io.Reader     { return s.stderr }
func (s *stubProcess) Close()                {}
func (s *stubProcess) Wait(context.Context) (int, error) {
	return s.code, s.waitErr
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

type errWriter struct{ err error }

func (e errWriter) Write([]byte) (int, error) { return 0, e.err }

// stubClient is a minimal sdkClient whose Exec returns a prepared process.
type stubClient struct {
	proc    Process
	execErr error
	closed  int
}

func (s *stubClient) Create(context.Context, string, []string, bool, float64, int, map[string]string) (Record, error) {
	return Record{}, nil
}
func (s *stubClient) List(context.Context) ([]Record, error)      { return nil, nil }
func (s *stubClient) Get(context.Context, string) (Record, error) { return Record{}, nil }
func (s *stubClient) Terminate(context.Context, string) error     { return nil }
func (s *stubClient) Close()                                      { s.closed++ }
func (s *stubClient) Exec(context.Context, string, []string, ExecParams) (Process, error) {
	return s.proc, s.execErr
}

func newStubProcess() *stubProcess {
	return &stubProcess{
		stdin:  &nopWriteCloser{w: io.Discard},
		stdout: strings.NewReader("out"),
		stderr: strings.NewReader(""),
	}
}

func TestRunQuiet(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	tests := []struct {
		name     string
		mutate   func(*stubProcess)
		execErr  error
		stdin    io.Reader
		wantErr  error
		wantExit int
		wantOut  string
	}{
		{name: "ok", wantOut: "out"},
		{name: "ok with stdin", stdin: strings.NewReader("in"), wantOut: "out"},
		{name: "exec error", execErr: boom, wantErr: boom},
		{name: "stdin copy error", stdin: errReader{boom}, wantErr: boom},
		{name: "stdin write error", stdin: strings.NewReader("in"), mutate: func(p *stubProcess) {
			p.stdin.w = errWriter{boom}
		}, wantErr: boom},
		{name: "stdin close error", mutate: func(p *stubProcess) { p.stdin.closeErr = boom }, wantErr: boom},
		{name: "stdout error", mutate: func(p *stubProcess) { p.stdout = errReader{boom} }, wantErr: boom},
		{name: "stderr error", mutate: func(p *stubProcess) { p.stderr = errReader{boom} }, wantErr: boom},
		{name: "wait error", mutate: func(p *stubProcess) { p.waitErr = boom }, wantErr: boom},
		{name: "non-zero exit", mutate: func(p *stubProcess) { p.code = 3 }, wantExit: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			proc := newStubProcess()
			if tt.mutate != nil {
				tt.mutate(proc)
			}
			c := &stubClient{proc: proc, execErr: tt.execErr}
			out, err := runQuiet(context.Background(), c, "box", []string{"x"}, tt.stdin)
			switch {
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
			case tt.wantExit != 0:
				exitErr, ok := errors.AsType[*sandbox.ExitError](err)
				if !ok || exitErr.Code != tt.wantExit {
					t.Fatalf("err = %v, want exit %d", err, tt.wantExit)
				}
			default:
				if err != nil || string(out) != tt.wantOut {
					t.Fatalf("out, err = %q, %v", out, err)
				}
			}
		})
	}
}

func TestWriteAndReadSandboxEnvErrors(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	c := &stubClient{execErr: boom}
	if err := writeSandboxEnv(context.Background(), c, "box", []string{"A=b"}); !errors.Is(err, boom) {
		t.Errorf("writeSandboxEnv err = %v, want boom", err)
	}
	if _, err := readSandboxEnv(context.Background(), c, "box"); !errors.Is(err, boom) {
		t.Errorf("readSandboxEnv err = %v, want boom", err)
	}
}

func TestPumpsCopyReportsErrorAndNilDst(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	p := &pumps{done: make(chan struct{})}
	p.copy(nil, strings.NewReader("discarded"))
	p.copy(io.Discard, errReader{boom})
	p.wg.Wait()
	p.mu.Lock()
	defer p.mu.Unlock()
	if !errors.Is(p.err, boom) {
		t.Fatalf("pump err = %v, want boom", p.err)
	}
}

func TestProviderMarkers(t *testing.T) {
	t.Parallel()
	p := New()
	p.SupportsCPU()
	p.SupportsMemory()
	if !p.SyncRequiresCreate() {
		t.Error("SyncRequiresCreate = false, want true")
	}
	if err := p.PrepareWorkspace(context.Background(), "box"); err != nil {
		t.Errorf("PrepareWorkspace: %v", err)
	}
}

const sec = "sec"

func TestSameCredentials(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                 string
		idA, secA, idB, secB string
		want                 bool
	}{
		{"equal", "id", sec, "id", sec, true},
		{"id differs", "id", sec, "id2", sec, false},
		{"secret differs", "id", sec, "id", "sec2", false},
		{"both differ", "a", "b", "c", "d", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := sameCredentials(tt.idA, tt.secA, tt.idB, tt.secB); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigureCredentials(t *testing.T) {
	t.Parallel()
	t.Run("missing", func(t *testing.T) {
		t.Parallel()
		for _, pair := range [][2]string{{"", ""}, {"id", ""}, {"", "secret"}} {
			if err := New().ConfigureCredentials(pair[0], pair[1]); err == nil {
				t.Errorf("ConfigureCredentials(%q, %q) = nil, want error", pair[0], pair[1])
			}
		}
	})
	t.Run("same credentials keep the client", func(t *testing.T) {
		t.Parallel()
		c := &stubClient{}
		p := NewWithClient(c, "id", "secret")
		if err := p.ConfigureCredentials("id", "secret"); err != nil {
			t.Fatalf("ConfigureCredentials: %v", err)
		}
		if p.client != sdkClient(c) || c.closed != 0 {
			t.Error("client was replaced or closed")
		}
	})
	t.Run("new credentials replace and close the client", func(t *testing.T) {
		t.Parallel()
		c := &stubClient{}
		p := NewWithClient(c, "id", "secret")
		if err := p.ConfigureCredentials("id2", "secret2"); err != nil {
			t.Skipf("SDK client could not be built offline: %v", err)
		}
		if c.closed != 1 {
			t.Errorf("old client closed %d times, want 1", c.closed)
		}
		if p.tokenID != "id2" || p.tokenSecret != "secret2" || p.client == nil {
			t.Error("credentials not stored")
		}
		p.client.Close()
	})
	t.Run("first configuration builds a client", func(t *testing.T) {
		t.Parallel()
		p := New()
		if err := p.ConfigureCredentials("id", "secret"); err != nil {
			t.Skipf("SDK client could not be built offline: %v", err)
		}
		p.client.Close()
	})
}

func TestTagsRoundTrip(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		box        string
		labels     []string
		wantTags   map[string]string
		wantLabels []string
	}{
		{"none", "a", nil, map[string]string{nameTag: "a"}, nil},
		{"labels sorted", "a", []string{"z", "b"}, map[string]string{nameTag: "a", "z": labelTagValue, "b": labelTagValue}, []string{"b", "z"}},
		{"skips empty and name tag", "a", []string{"", nameTag, "x"}, map[string]string{nameTag: "a", "x": labelTagValue}, []string{"x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tags := tagsFor(tt.box, tt.labels)
			if !reflect.DeepEqual(tags, tt.wantTags) {
				t.Fatalf("tagsFor = %v, want %v", tags, tt.wantTags)
			}
			name, labels := fromTags(tags)
			if name != tt.box || !reflect.DeepEqual(labels, tt.wantLabels) {
				t.Errorf("fromTags = %q, %v; want %q, %v", name, labels, tt.box, tt.wantLabels)
			}
		})
	}
	if name, labels := fromTags(nil); name != "" || labels != nil {
		t.Errorf("fromTags(nil) = %q, %v", name, labels)
	}
}

func TestNotFound(t *testing.T) {
	t.Parallel()
	other := errors.New("other")
	if got := notFound(sdk.NotFoundError{Exception: "gone"}); !errors.Is(got, ErrNotFound) {
		t.Errorf("notFound(sdk) = %v, want ErrNotFound", got)
	}
	if got := notFound(other); !errors.Is(got, other) {
		t.Errorf("notFound(other) = %v, want other", got)
	}
}

func TestDefaultIndexPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	got, err := defaultIndexPath()
	if err != nil {
		t.Fatalf("defaultIndexPath: %v", err)
	}
	if filepath.Base(got) != indexFile || filepath.Base(filepath.Dir(got)) != "kaxa" {
		t.Errorf("path = %q", got)
	}
}

func TestDefaultIndexPathNoConfigDir(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("AppData", "")
	if _, err := defaultIndexPath(); err == nil {
		t.Skip("platform resolves a config dir without HOME")
	}
}

func TestSnapshotIndexDefaultPathUsed(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	p := New()
	got, err := p.ListSnapshots(context.Background(), "box")
	if err != nil || len(got) != 0 {
		t.Fatalf("ListSnapshots = %v, %v", got, err)
	}
	all, err := p.ListAllSnapshots(context.Background())
	if err != nil || len(all) != 0 {
		t.Fatalf("ListAllSnapshots = %v, %v", all, err)
	}
}

func TestSnapshotIndexSaveErrors(t *testing.T) {
	t.Parallel()
	t.Run("directory is a file", func(t *testing.T) {
		t.Parallel()
		blocker := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		idx := newSnapshotIndex(filepath.Join(blocker, "sub", "idx.json"))
		if err := idx.save(nil); err == nil {
			t.Error("save = nil, want error")
		}
	})
	t.Run("rename onto directory", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		target := filepath.Join(dir, "idx.json")
		if err := os.Mkdir(target, 0o700); err != nil {
			t.Fatal(err)
		}
		idx := newSnapshotIndex(target)
		if err := idx.save([]snapshotEntry{{ID: "a"}}); err == nil {
			t.Error("save = nil, want error")
		}
		left, _ := filepath.Glob(filepath.Join(dir, ".modal-snapshots-*"))
		if len(left) != 0 {
			t.Errorf("temporary files left: %v", left)
		}
	})
}

func TestSnapshotIndexLoadErrorsPropagate(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "idx.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	idx := newSnapshotIndex(path)
	if err := idx.add(snapshotEntry{ID: "a"}); err == nil {
		t.Error("add = nil, want error")
	}
	if err := idx.remove("a"); err == nil {
		t.Error("remove = nil, want error")
	}
	if _, err := idx.all(); err == nil {
		t.Error("all = nil, want error")
	}
	if _, _, err := idx.find("s", "a"); err == nil {
		t.Error("find = nil, want error")
	}
	p := New()
	p.SetSnapshotIndexPath(path)
	if _, err := p.ListSnapshots(context.Background(), "box"); err == nil {
		t.Error("ListSnapshots = nil, want error")
	}
	if _, err := p.ListAllSnapshots(context.Background()); err == nil {
		t.Error("ListAllSnapshots = nil, want error")
	}
	if _, err := p.ListSnapshots(context.Background(), "Bad Name!"); err == nil {
		t.Error("ListSnapshots invalid name = nil, want error")
	}
}

// snapStub is a stubClient that also serves snapshots.
type snapStub struct {
	stubClient
	deleteErr error
	deleted   []string
}

func (s *snapStub) SnapshotFilesystem(context.Context, string) (string, error) { return "im-1", nil }

func (s *snapStub) CreateFromImage(context.Context, string, []string, string) (Record, error) {
	return Record{}, nil
}

func (s *snapStub) DeleteImage(_ context.Context, id string) error {
	s.deleted = append(s.deleted, id)
	return s.deleteErr
}

func TestDeleteSnapshotPaths(t *testing.T) {
	t.Parallel()
	seed := func(t *testing.T, c sdkClient) *Provider {
		t.Helper()
		p := NewWithClient(c, "id", "secret")
		p.SetSnapshotIndexPath(filepath.Join(t.TempDir(), "idx.json"))
		idx, err := p.snapshotIndex()
		if err != nil {
			t.Fatal(err)
		}
		if err := idx.add(snapshotEntry{ID: "im-1", Sandbox: "box", CreatedAt: idx.now()}); err != nil {
			t.Fatal(err)
		}
		return p
	}
	t.Run("deleted", func(t *testing.T) {
		t.Parallel()
		c := &snapStub{}
		p := seed(t, c)
		if err := p.DeleteSnapshot(context.Background(), "box", "im-1"); err != nil {
			t.Fatalf("DeleteSnapshot: %v", err)
		}
		left, _ := p.ListSnapshots(context.Background(), "box")
		if len(left) != 0 {
			t.Errorf("snapshots left: %v", left)
		}
	})
	t.Run("image already gone", func(t *testing.T) {
		t.Parallel()
		p := seed(t, &snapStub{deleteErr: ErrImageNotFound})
		if err := p.DeleteSnapshot(context.Background(), "box", "im-1"); err != nil {
			t.Fatalf("DeleteSnapshot: %v", err)
		}
	})
	t.Run("delete fails", func(t *testing.T) {
		t.Parallel()
		p := seed(t, &snapStub{deleteErr: errors.New("api down")})
		if err := p.DeleteSnapshot(context.Background(), "box", "im-1"); err == nil {
			t.Error("DeleteSnapshot = nil, want error")
		}
	})
	t.Run("unknown snapshot", func(t *testing.T) {
		t.Parallel()
		p := seed(t, &snapStub{})
		if err := p.DeleteSnapshot(context.Background(), "box", "im-9"); err == nil {
			t.Error("DeleteSnapshot = nil, want error")
		}
	})
	t.Run("client without snapshots", func(t *testing.T) {
		t.Parallel()
		p := seed(t, &stubClient{})
		if err := p.DeleteSnapshot(context.Background(), "box", "im-1"); err == nil {
			t.Error("DeleteSnapshot = nil, want error")
		}
	})
	t.Run("no credentials", func(t *testing.T) {
		t.Parallel()
		if err := New().DeleteSnapshot(context.Background(), "box", "im-1"); err == nil {
			t.Error("DeleteSnapshot = nil, want error")
		}
	})
}

func TestRealClientFailsFastOnCanceledContext(t *testing.T) {
	t.Parallel()
	c, err := newSDKClient("ak-test", "as-test")
	if err != nil {
		t.Skipf("SDK client could not be built offline: %v", err)
	}
	defer c.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if rec, err := c.Create(ctx, "box", []string{"a"}, true, 1, 512, nil); err == nil {
		t.Errorf("Create = %v, want error", rec)
	}
	if _, err := c.Create(ctx, "box", nil, false, 0, 0, nil); err == nil {
		t.Error("Create without tools = nil, want error")
	}
	if _, err := c.List(ctx); err == nil {
		t.Error("List = nil, want error")
	}
	if _, err := c.Get(ctx, "box"); err == nil {
		t.Error("Get = nil, want error")
	}
	if err := c.Terminate(ctx, "box"); err == nil {
		t.Error("Terminate = nil, want error")
	}
	if _, err := c.Exec(ctx, "box", []string{"true"}, ExecParams{}); err == nil {
		t.Error("Exec = nil, want error")
	}
	if _, err := c.SnapshotFilesystem(ctx, "box"); err == nil {
		t.Error("SnapshotFilesystem = nil, want error")
	}
	if _, err := c.CreateFromImage(ctx, "box", nil, "im-1"); err == nil {
		t.Error("CreateFromImage = nil, want error")
	}
	if err := c.DeleteImage(ctx, "im-1"); err == nil {
		t.Error("DeleteImage = nil, want error")
	}
}

func TestSDKProcessAccessors(t *testing.T) {
	t.Parallel()
	p := &sdkProcess{cp: &sdk.ContainerProcess{}}
	if p.Stdin() != nil || p.Stdout() != nil || p.Stderr() != nil {
		t.Error("zero ContainerProcess should expose nil streams")
	}
}
