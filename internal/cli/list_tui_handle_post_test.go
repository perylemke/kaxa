package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/sandbox"
)

type destroyRecorder struct {
	idleProvider
	destroyed sandbox.ID
}

func (p *destroyRecorder) Destroy(_ context.Context, id sandbox.ID) error {
	p.destroyed = id
	return nil
}

type restartRecorder struct {
	idleProvider
	id sandbox.ID
}

func (p *restartRecorder) Restart(_ context.Context, id sandbox.ID) error {
	p.id = id
	return nil
}

type upgradeRecorder struct {
	idleProvider
	id sandbox.ID
}

func (p *upgradeRecorder) Upgrade(_ context.Context, id sandbox.ID) error {
	p.id = id
	return nil
}

func TestHandleListPostActionStreamsRestart(t *testing.T) {
	t.Parallel()

	p := &restartRecorder{}
	err := handleListPostActionStreams(
		context.Background(),
		strings.NewReader("y\n"),
		new(bytes.Buffer),
		new(bytes.Buffer),
		map[string]sandbox.Provider{p.Name(): p},
		listTUIResult{postAction: listPostRestart, id: fixtureDemo, provider: p.Name()},
	)
	if err != nil || p.id != fixtureDemo {
		t.Fatalf("err=%v id=%q", err, p.id)
	}
}

func TestHandleListPostActionStreamsUpgrade(t *testing.T) {
	t.Parallel()

	p := &upgradeRecorder{}
	err := handleListPostActionStreams(
		context.Background(),
		strings.NewReader("y\n"),
		new(bytes.Buffer),
		new(bytes.Buffer),
		map[string]sandbox.Provider{p.Name(): p},
		listTUIResult{postAction: listPostUpgrade, id: fixtureDemo, provider: p.Name()},
	)
	if err != nil || p.id != fixtureDemo {
		t.Fatalf("err=%v id=%q", err, p.id)
	}
}

func TestHandleListPostActionStreamsDestroy(t *testing.T) {
	t.Parallel()

	p := &destroyRecorder{}
	err := handleListPostActionStreams(
		context.Background(),
		strings.NewReader("y\n"),
		new(bytes.Buffer),
		new(bytes.Buffer),
		map[string]sandbox.Provider{p.Name(): p},
		listTUIResult{postAction: listPostDestroy, id: fixtureDemo, provider: p.Name()},
	)
	if err != nil || p.destroyed != fixtureDemo {
		t.Fatalf("err=%v id=%q", err, p.destroyed)
	}
}

func TestHandleListPostActionStreamsNoOpWhenNone(t *testing.T) {
	t.Parallel()

	err := handleListPostActionStreams(
		context.Background(),
		strings.NewReader("y\n"),
		new(bytes.Buffer),
		new(bytes.Buffer),
		map[string]sandbox.Provider{},
		listTUIResult{postAction: listPostNone, id: ""},
	)
	if err != nil {
		t.Fatal(err)
	}
}
