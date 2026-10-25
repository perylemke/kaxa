package sprites_test

import (
	"bytes"
	"context"
	"io"
	"reflect"
	"testing"

	"github.com/perylemke/sbox/internal/provider/sprites"
	"github.com/perylemke/sbox/internal/sandbox"
)

func TestSyncWorkspaceExtractsArchive(t *testing.T) {
	t.Parallel()

	const spriteID = "box"
	payload := []byte("tar-bytes")
	var calls [][]string
	var stdin []byte
	client := &fakeSDKClient{
		run: func(_ context.Context, name string, argv []string, in io.Reader, _, _ io.Writer) error {
			if name != spriteID {
				t.Errorf("sprite = %q", name)
			}
			calls = append(calls, append([]string(nil), argv...))
			if in == nil {
				return nil
			}
			body, err := io.ReadAll(in)
			if err != nil {
				t.Errorf("read stdin: %v", err)
			}
			stdin = body
			return nil
		},
	}
	p := sprites.NewWithClient(client, "token")
	var _ sandbox.WorkspaceSyncer = p

	err := p.SyncWorkspace(context.Background(), spriteID, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("SyncWorkspace: %v", err)
	}
	want := [][]string{
		{"mkdir", "-p", "/home/sprite/workspace"},
		{"tar", "-xzf", "-", "-C", "/home/sprite/workspace"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v", calls)
	}
	if !bytes.Equal(stdin, payload) {
		t.Fatalf("stdin = %q", stdin)
	}
}
