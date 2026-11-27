package vercel

import (
	"context"
	"testing"
	"time"
)

func TestRestart(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		lag  int
	}{{"stops at once", 0}, {"waits for stopping", 3}} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, f := newTestProvider(t)
			f.lagStop = tt.lag
			createBox(t, p)
			first := f.sandboxes[boxName].session
			if err := p.Restart(context.Background(), boxName); err != nil {
				t.Fatal(err)
			}
			sb := f.sandboxes[boxName]
			if sb.status != statusRunning || sb.session == first {
				t.Fatalf("sandbox = %+v", sb)
			}
		})
	}
}

func TestRestartStoppedSandbox(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	createBox(t, p)
	f.mu.Lock()
	f.sandboxes[boxName].status = statusStopped
	f.mu.Unlock()
	if err := p.Restart(context.Background(), boxName); err != nil {
		t.Fatal(err)
	}
	if f.calls("POST /v2/sandboxes/sessions/{sid}/stop") != 0 || f.sandboxes[boxName].status != statusRunning {
		t.Fatal("a stopped sandbox was stopped again or not resumed")
	}
}

func TestRestartFailures(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	t.Run("never stops", func(t *testing.T) {
		t.Parallel()
		p, f := newTestProvider(t)
		f.lagStop = 1000
		createBox(t, p)
		p.SetReadiness(30*time.Millisecond, time.Millisecond)
		if err := p.Restart(ctx, boxName); err == nil {
			t.Fatal("a sandbox that never stopped was resumed")
		}
	})
	t.Run("stop refused", func(t *testing.T) {
		t.Parallel()
		p, f := newTestProvider(t)
		createBox(t, p)
		f.fail2("POST /v2/sandboxes/sessions/{sid}/stop", 500)
		if err := p.Restart(ctx, boxName); err == nil {
			t.Fatal("stop failure ignored")
		}
	})
	t.Run("canceled while stopping", func(t *testing.T) {
		t.Parallel()
		p, f := newTestProvider(t)
		f.lagStop = 1000
		createBox(t, p)
		p.SetReadiness(time.Minute, 20*time.Millisecond)
		cctx, cancel := context.WithTimeout(ctx, 60*time.Millisecond)
		defer cancel()
		if err := p.Restart(cctx, boxName); err == nil {
			t.Fatal("canceled restart returned no error")
		}
	})
}

func TestReadinessDefaults(t *testing.T) {
	t.Parallel()
	timeout, interval := New().readiness()
	if timeout != defaultReadyTimeout || interval != defaultReadyInterval {
		t.Fatalf("readiness() = %v, %v", timeout, interval)
	}
}
