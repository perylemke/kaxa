package docker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

var (
	boxSnapshotName = "kaxa-box-" + testSnapshotAt
	snapshotNameRe  = regexp.MustCompile(`^kaxa-box-\d{14}$`)
	snapshotBase    = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
)

const (
	targetName      = "copy"
	snapOne         = "s1"
	foreignSnapshot = "backup-before-upgrade"
	otherBox        = "other"
)

// userErrorOf returns the user error in err, which must mention msg.
func userErrorOf(t *testing.T, err error, msg string) *usererr.Error {
	t.Helper()
	ue, ok := usererr.As(err)
	if !ok || !strings.Contains(ue.Error(), msg) {
		t.Fatalf("err = %v, want a user error containing %q", err, msg)
	}
	return ue
}

func wantUserError(t *testing.T, err error, msg string) {
	t.Helper()
	_ = userErrorOf(t, err, msg)
}

func TestRestoreModeIsNewSandbox(t *testing.T) {
	t.Parallel()
	if got := New().RestoreMode(); got != sandbox.RestoreNewSandbox {
		t.Fatalf("RestoreMode() = %q", got)
	}
}

func TestSnapshotSavesTheSandboxAndWaitsUntilReady(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	createBox(t, p)
	snap, err := p.Snapshot(context.Background(), testSandbox, "before the migration")
	if err != nil {
		t.Fatal(err)
	}
	if snap.ID != "snap1" || snap.Sandbox != testSandbox || snap.Provider != ProviderName || snap.Comment != "before the migration" {
		t.Fatalf("snapshot = %+v", snap)
	}
	if got := api.snapState().created; len(got) != 1 || !snapshotNameRe.MatchString(got[0]) {
		t.Fatalf("display names = %q", got)
	}
	if api.seen("get-snapshot") < 1 {
		t.Fatal("the snapshot was not polled until ready")
	}
}

func TestSnapshotStartsAStoppedSandbox(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	createBox(t, p)
	api.setStatus(testSandbox, statusStopped)
	if _, err := p.Snapshot(context.Background(), testSandbox, ""); err != nil {
		t.Fatal(err)
	}
	if api.seen("start") != 1 {
		t.Fatalf("start calls = %d, want 1", api.seen("start"))
	}
}

func TestSnapshotRejectsBadInput(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	createBox(t, p)
	ctx := context.Background()
	tests := []struct {
		name    string
		id      sandbox.ID
		comment string
		want    string
	}{
		{"control character", testSandbox, "a\nb", "control character"},
		{"too long", testSandbox, strings.Repeat("x", 300), "longer than"},
		{"invalid name", "Bad_Name", "", "invalid sandbox name"},
		{"unknown sandbox", otherBox, "", "not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := p.Snapshot(ctx, tt.id, tt.comment)
			wantUserError(t, err, tt.want)
		})
	}
	if api.seen("create-snapshot") != 0 {
		t.Fatal("a rejected request must not reach the API")
	}
}

func TestSnapshotFailureIsReportedAndRemoved(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	createBox(t, p)
	api.snapState().broken = true
	_, err := p.Snapshot(context.Background(), testSandbox, "")
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("err = %v, want the reason Docker gave", err)
	}
	if api.seen("delete-snapshot") != 1 || len(api.snapState().snaps) != 0 {
		t.Fatalf("the failed snapshot must be deleted: %d delete calls, %d left", api.seen("delete-snapshot"), len(api.snapState().snaps))
	}
}

func TestSnapshotTimesOut(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	p := NewWithClients(api, &fakeStarter{}, time.Millisecond, 50*time.Millisecond)
	createBox(t, p)
	api.snapState().stuck = true
	_, err := p.Snapshot(context.Background(), testSandbox, "")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want a deadline error", err)
	}
}

func TestSnapshotMapsAPIErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"denied", &APIError{Status: 403, Code: codePermissionDenied}, "denied access to snapshots"},
		{"name taken", ErrSnapshotExists, "already exists"},
		{"quota", &APIError{Status: 429, Code: codeExhausted, Message: "too many"}, "limit reached"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, api, _ := newTestProvider(t)
			createBox(t, p)
			api.failOn["create-snapshot"] = tt.err
			_, err := p.Snapshot(context.Background(), testSandbox, "")
			wantUserError(t, err, tt.want)
		})
	}
}

func TestSnapshotDeniedHintNamesTheScopes(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	createBox(t, p)
	api.failOn["create-snapshot"] = &APIError{Status: 403, Code: codePermissionDenied}
	_, err := p.Snapshot(context.Background(), testSandbox, "")
	ue := userErrorOf(t, err, "denied access to snapshots")
	if !strings.Contains(ue.Hint, "snapshotsWrite") {
		t.Fatalf("hint = %q", ue.Hint)
	}
}

func TestSnapshotWithoutCredentials(t *testing.T) {
	t.Parallel()
	_, err := New().Snapshot(context.Background(), testSandbox, "")
	if !errors.Is(err, sandbox.ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}

func TestListSnapshotsReturnsOnlyKaxaSnapshotsOfTheSandbox(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	api.addSnapshot(snapOne, boxSnapshotName, "first", snapshotBase)
	api.addSnapshot("s2", "kaxa-box-20260102030406", "second", snapshotBase.Add(time.Minute))
	api.addSnapshot("s3", "kaxa-other-"+testSnapshotAt, "", snapshotBase)
	api.addSnapshot("s4", foreignSnapshot, "mine", snapshotBase)

	got, err := p.ListSnapshots(context.Background(), testSandbox)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "s2" || got[1].ID != snapOne {
		t.Fatalf("snapshots = %+v, want s2 then s1", got)
	}
	// The list view has no comment, so the provider reads it from the detail.
	if got[0].Comment != "second" || got[1].Comment != "first" || got[0].Sandbox != testSandbox {
		t.Fatalf("snapshots = %+v", got)
	}
}

func TestListAllSnapshotsIncludesDestroyedSandboxes(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	api.addSnapshot(snapOne, boxSnapshotName, "", snapshotBase)
	api.addSnapshot("s3", "kaxa-gone-"+testSnapshotAt, "", snapshotBase.Add(time.Hour))
	api.addSnapshot("s4", foreignSnapshot, "", snapshotBase)

	got, err := p.ListAllSnapshots(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Sandbox != "gone" || got[1].Sandbox != testSandbox {
		t.Fatalf("snapshots = %+v", got)
	}
}

func TestListAllSnapshotsSkipsDeletedAndKeepsUnreadableDetails(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	api.addSnapshot(snapOne, boxSnapshotName, "kept", snapshotBase)
	api.addSnapshot("s2", "kaxa-box-20260102030406", "", snapshotBase)
	api.snapState().snaps["snapshots/s2"].Status = snapshotDeleted
	api.failOn["get-snapshot"] = errors.New("boom")

	got, err := p.ListAllSnapshots(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != snapOne || got[0].Comment != "" {
		t.Fatalf("snapshots = %+v, want s1 without a comment", got)
	}
}

func TestListSnapshotsErrors(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	if _, err := p.ListSnapshots(context.Background(), "Bad_Name"); err == nil {
		t.Fatal("an invalid name must be rejected")
	}
	api.failOn["list-snapshots"] = &APIError{Status: 403, Code: codePermissionDenied}
	_, err := p.ListSnapshots(context.Background(), testSandbox)
	wantUserError(t, err, "denied access to snapshots")
	if _, err := New().ListAllSnapshots(context.Background()); !errors.Is(err, sandbox.ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}

func TestRestoreCreatesTheTargetAndKeepsTheSource(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	createBox(t, p)
	api.addSnapshot(snapOne, boxSnapshotName, "", snapshotBase)
	id, err := p.Restore(context.Background(), sandbox.RestoreRequest{Sandbox: testSandbox, Snapshot: snapOne, Target: targetName})
	if err != nil || id != targetName {
		t.Fatalf("id = %q, err = %v", id, err)
	}
	if got := api.snapState().restored; len(got) != 1 || got[0] != targetName {
		t.Fatalf("restored = %q", got)
	}
	infos, err := p.List(context.Background())
	if err != nil || len(infos) != 2 {
		t.Fatalf("sandboxes = %+v, err = %v; the source must be kept", infos, err)
	}
	if infos[0].Status != statusRunning || infos[1].Status != statusRunning {
		t.Fatalf("sandboxes = %+v, want both running", infos)
	}
}

func TestRestoreWorksAfterTheSourceWasDestroyed(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	api.addSnapshot(snapOne, boxSnapshotName, "", snapshotBase)
	if _, err := p.Restore(context.Background(), sandbox.RestoreRequest{Sandbox: testSandbox, Snapshot: snapOne, Target: targetName}); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreRejectsWhatIsNotTheSandboxsOwnSnapshot(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	api.addSnapshot(snapOne, boxSnapshotName, "", snapshotBase)
	api.addSnapshot("s2", foreignSnapshot, "", snapshotBase)
	api.addSnapshot("s3", "kaxa-other-"+testSnapshotAt, "", snapshotBase)
	for _, snap := range []sandbox.SnapshotID{"s2", "s3", "missing"} {
		_, err := p.Restore(context.Background(), sandbox.RestoreRequest{Sandbox: testSandbox, Snapshot: snap, Target: targetName})
		ue := userErrorOf(t, err, "not found for sandbox")
		if !strings.Contains(ue.Hint, "snapshot list box") {
			t.Errorf("%s: hint = %q", snap, ue.Hint)
		}
	}
	if api.seen("restore-snapshot") != 0 {
		t.Fatal("a snapshot kaxa did not take of that sandbox must not be restored")
	}
}

func TestRestoreRejectsBadRequests(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	createBox(t, p)
	api.addSnapshot(snapOne, boxSnapshotName, "", snapshotBase)
	tests := []struct {
		name string
		req  sandbox.RestoreRequest
		want string
	}{
		{"invalid target", sandbox.RestoreRequest{Sandbox: testSandbox, Snapshot: snapOne, Target: "Bad_Name"}, "invalid sandbox name"},
		{"same target", sandbox.RestoreRequest{Sandbox: testSandbox, Snapshot: snapOne, Target: testSandbox}, "must differ"},
		{"invalid snapshot", sandbox.RestoreRequest{Sandbox: testSandbox, Snapshot: "../x", Target: targetName}, "invalid snapshot id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := p.Restore(context.Background(), tt.req)
			wantUserError(t, err, tt.want)
		})
	}
}

func TestRestoreRejectsATakenTargetName(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	createBox(t, p)
	api.addSnapshot(snapOne, "kaxa-src-"+testSnapshotAt, "", snapshotBase)
	_, err := p.Restore(context.Background(), sandbox.RestoreRequest{Sandbox: "src", Snapshot: snapOne, Target: testSandbox})
	wantUserError(t, err, "already exists")
	if api.seen("restore-snapshot") != 0 {
		t.Fatal("a taken target must not reach the restore call")
	}
}

func TestRestoreRejectsASnapshotThatIsNotReady(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	api.addSnapshot(snapOne, boxSnapshotName, "", snapshotBase)
	api.snapState().snaps["snapshots/s1"].Status = snapshotCreating
	_, err := p.Restore(context.Background(), sandbox.RestoreRequest{Sandbox: testSandbox, Snapshot: snapOne, Target: targetName})
	wantUserError(t, err, "creating")
}

func TestRestoreRecordsASandboxWithoutLabels(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	createBox(t, p, labelKaxa, sandbox.SyncLabel)
	if _, err := p.Snapshot(context.Background(), testSandbox, ""); err != nil {
		t.Fatal(err)
	}
	api.snapState().unlabeled = true
	if _, err := p.Restore(context.Background(), sandbox.RestoreRequest{Sandbox: testSandbox, Snapshot: "snap1", Target: targetName}); err != nil {
		t.Fatal(err)
	}
	if api.seen("delete") != 0 {
		t.Fatal("a restored sandbox that kaxa can record must be kept")
	}
	known, err := p.restoredLabels()
	if err != nil {
		t.Fatal(err)
	}
	var resource string
	for res, r := range api.recs {
		if r.Name == targetName {
			resource = res
		}
	}
	if got := known[resource]; !slices.Equal(got, []string{labelKaxa, sandbox.SyncLabel}) {
		t.Fatalf("recorded labels = %v, want those of the source sandbox", got)
	}
}

func TestRestoreSkipsTheRecordWhenDockerKeepsTheLabels(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	api.addSnapshot(snapOne, boxSnapshotName, "", snapshotBase)
	if _, err := p.Restore(context.Background(), sandbox.RestoreRequest{Sandbox: testSandbox, Snapshot: snapOne, Target: targetName}); err != nil {
		t.Fatal(err)
	}
	known, err := p.restoredLabels()
	if err != nil || len(known) != 0 {
		t.Fatalf("known = %v, err = %v; a sandbox with the labels needs no record", known, err)
	}
}

func TestRestoreRemovesTheSandboxWhenItCannotBeRecorded(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	api.addSnapshot(snapOne, boxSnapshotName, "", snapshotBase)
	api.snapState().unlabeled = true
	// A file where the config directory should be makes the index unwritable.
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	p.SetRestoredIndexPath(filepath.Join(blocker, restoredFile))
	_, err := p.Restore(context.Background(), sandbox.RestoreRequest{Sandbox: testSandbox, Snapshot: snapOne, Target: targetName})
	wantUserError(t, err, "could not record the restored sandbox")
	if api.seen("delete") != 1 || len(api.snapState().snaps) != 1 {
		t.Fatalf("delete calls = %d, snapshots = %d; the sandbox must go and the snapshot stay", api.seen("delete"), len(api.snapState().snaps))
	}
}

func TestDestroyForgetsARestoredSandbox(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	api.addSnapshot(snapOne, boxSnapshotName, "", snapshotBase)
	api.snapState().unlabeled = true
	if _, err := p.Restore(context.Background(), sandbox.RestoreRequest{Sandbox: testSandbox, Snapshot: snapOne, Target: targetName}); err != nil {
		t.Fatal(err)
	}
	if err := p.Destroy(context.Background(), targetName); err != nil {
		t.Fatal(err)
	}
	if known, err := p.restoredLabels(); err != nil || len(known) != 0 {
		t.Fatalf("known = %v, err = %v; destroy must drop the record", known, err)
	}
}

func TestSnapshotStoresTheLabelsAndTheComment(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	createBox(t, p, labelKaxa, sandbox.SyncLabel)
	if _, err := p.Snapshot(context.Background(), testSandbox, "before"); err != nil {
		t.Fatal(err)
	}
	stored := api.snapState().snaps["snapshots/snap1"]
	meta := stored.meta()
	if meta.Comment != "before" || !slices.Equal(meta.Labels, []string{labelKaxa, sandbox.SyncLabel}) {
		t.Fatalf("description = %q, meta = %+v", stored.Description, meta)
	}
}

func TestRestoreReadsTheLabelsFromTheSandboxNotFromTheAnswer(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	api.addSnapshot(snapOne, boxSnapshotName, "", snapshotBase)
	api.snapState().partial = true
	id, err := p.Restore(context.Background(), sandbox.RestoreRequest{Sandbox: testSandbox, Snapshot: snapOne, Target: targetName})
	if err != nil || id != targetName {
		t.Fatalf("id = %q, err = %v; the labels of the sandbox itself must decide", id, err)
	}
	if api.seen("delete") != 0 {
		t.Fatal("a sandbox with the labels must be kept")
	}
}

func TestRestoreRemovesASandboxThatNeverRuns(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	api.addSnapshot(snapOne, boxSnapshotName, "", snapshotBase)
	api.failOn["get"] = errors.New("boom")
	_, err := p.Restore(context.Background(), sandbox.RestoreRequest{Sandbox: testSandbox, Snapshot: snapOne, Target: targetName})
	if err == nil || api.seen("delete") != 1 {
		t.Fatalf("err = %v, delete calls = %d", err, api.seen("delete"))
	}
}

func TestRestoreMapsAPIErrors(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	api.addSnapshot(snapOne, boxSnapshotName, "", snapshotBase)
	api.failOn["restore-snapshot"] = &APIError{Status: 403, Code: codePermissionDenied}
	_, err := p.Restore(context.Background(), sandbox.RestoreRequest{Sandbox: testSandbox, Snapshot: snapOne, Target: targetName})
	wantUserError(t, err, "denied access to snapshots")
}

func TestDeleteSnapshotRemovesIt(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	api.addSnapshot(snapOne, boxSnapshotName, "", snapshotBase)
	if err := p.DeleteSnapshot(context.Background(), testSandbox, snapOne); err != nil {
		t.Fatal(err)
	}
	if len(api.snapState().snaps) != 0 {
		t.Fatal("the snapshot was not deleted")
	}
}

func TestDeleteSnapshotLeavesWhatKaxaDidNotTake(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	api.addSnapshot(snapOne, foreignSnapshot, "", snapshotBase)
	api.addSnapshot("s2", "kaxa-other-"+testSnapshotAt, "", snapshotBase)
	for _, snap := range []sandbox.SnapshotID{snapOne, "s2", "missing", "../x"} {
		if err := p.DeleteSnapshot(context.Background(), testSandbox, snap); err == nil {
			t.Errorf("DeleteSnapshot(%q) must fail", snap)
		}
	}
	if api.seen("delete-snapshot") != 0 || len(api.snapState().snaps) != 2 {
		t.Fatal("a snapshot kaxa did not take of that sandbox must stay")
	}
}

func TestDeleteSnapshotAlreadyGoneSucceeds(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	api.addSnapshot(snapOne, boxSnapshotName, "", snapshotBase)
	api.failOn["delete-snapshot"] = ErrSnapshotNotFound
	if err := p.DeleteSnapshot(context.Background(), testSandbox, snapOne); err != nil {
		t.Fatalf("err = %v, want success for a snapshot that is already gone", err)
	}
}

func TestDeleteSnapshotMapsAPIErrors(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	api.addSnapshot(snapOne, boxSnapshotName, "", snapshotBase)
	api.failOn["delete-snapshot"] = &APIError{Status: 403, Code: codePermissionDenied}
	wantUserError(t, p.DeleteSnapshot(context.Background(), testSandbox, snapOne), "denied access to snapshots")
}

func TestDeleteSnapshotExplainsAConflict(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	api.addSnapshot(snapOne, boxSnapshotName, "", snapshotBase)
	api.failOn["delete-snapshot"] = &APIError{Status: 409, Code: "failedPrecondition", Message: "Template has 1 running sandboxes " + testPAT}
	err := p.DeleteSnapshot(context.Background(), testSandbox, snapOne)
	ue := userErrorOf(t, err, "cannot delete snapshot")
	if !strings.Contains(ue.Error(), "1 running sandboxes") || strings.Contains(ue.Error(), testPAT) {
		t.Fatalf("message = %q, want the reason from Docker, without credentials", ue.Error())
	}
	if !strings.Contains(ue.Hint, "destroy it first") {
		t.Fatalf("hint = %q", ue.Hint)
	}
}
