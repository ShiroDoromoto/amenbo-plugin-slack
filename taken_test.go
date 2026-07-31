package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// oneProject is the project a test's runs reach unless it says otherwise. amenbo launches a plugin
// for the project whose event fired and names it by ref, so a test that says nothing about it is
// still a test of one project.
const oneProject = "AMB-P-1"

// remembers points the plugin at a store of its own for the length of one test, so what it writes
// down lands in a temp directory rather than in anyone's app-data.
func remembers(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv(storeEnv, home)
	t.Setenv(reachEnv, oneProject)
	return home
}

// stateDir is where the runs reaching one project keep what they remember between them.
func stateDir(home, project string) string {
	return filepath.Join(home, "plugins", pluginName, project)
}

// moment is one write, at one moment: the payload amenbo delivers, and delivers again if a runner
// died before it could take the row off the queue.
func moment(event, at string) input {
	in := aiWrite(event)
	in.At = at
	return in
}

// The same event delivered twice is one message. Nobody on the receiving end could tell the second
// from a real one, so the second delivery is where it stops.
func TestHookSendsOneMessageForAnEventDeliveredTwice(t *testing.T) {
	remembers(t)
	posted := slackStands(t)
	readsBack(t, "AMB-T-42", "Ship the thing")

	replayed := moment(eventTaskDone, "2026-07-22T09:00:00Z")
	for range 3 {
		if err := hook(replayed); err != nil {
			t.Fatal(err)
		}
	}

	if len(*posted) != 1 {
		t.Errorf("a replay is not news, got %v", *posted)
	}
}

// What is not stopped is the user doing the same thing twice: two writes are two moments, and both
// are worth hearing about.
func TestHookReportsTheSameEventAtAnotherMoment(t *testing.T) {
	remembers(t)
	posted := slackStands(t)
	readsBack(t, "AMB-T-42", "Ship the thing")

	if err := hook(moment(eventStatusChanged, "2026-07-22T09:00:00Z")); err != nil {
		t.Fatal(err)
	}
	if err := hook(moment(eventStatusChanged, "2026-07-22T09:00:01Z")); err != nil {
		t.Fatal(err)
	}

	if len(*posted) != 2 {
		t.Errorf("two writes are two messages, got %v", *posted)
	}
}

// The record lives in the plugin's own installed directory, under the project it is about — so
// removing the plugin takes it away with everything else it left behind, and a second project on
// the same store is remembered apart from this one.
func TestTheRecordLivesWithTheInstalledPluginUnderItsProject(t *testing.T) {
	home := remembers(t)
	slackStands(t)
	readsBack(t, "AMB-T-42", "Ship the thing")

	if err := hook(moment(eventTaskDone, "2026-07-22T09:00:00Z")); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(stateDir(home, oneProject), takenFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("nothing was written down at %s: %v", path, err)
	}
	if want := "task.done 42 2026-07-22T09:00:00Z"; !strings.Contains(string(raw), want) {
		t.Errorf("the key should name what happened, to which record, when: %q", raw)
	}
}

// The record is a bounded tail: it has to outlast a burst of deliveries, not the history of the
// store, so it stops growing rather than stopping the plugin.
func TestTheRecordKeepsOnlyItsTail(t *testing.T) {
	home := remembers(t)
	slackStands(t)
	readsBack(t, "AMB-T-42", "Ship the thing")

	for i := range remembered + 10 {
		if err := hook(moment(eventTaskDone, fmt.Sprintf("2026-07-22T09:%02d:%02d", i/60, i%60))); err != nil {
			t.Fatal(err)
		}
	}

	raw, err := os.ReadFile(filepath.Join(stateDir(home, oneProject), takenFile))
	if err != nil {
		t.Fatal(err)
	}
	kept := strings.Count(strings.TrimSpace(string(raw)), "\n") + 1
	if kept != remembered {
		t.Errorf("the tail should hold %d keys, got %d", remembered, kept)
	}
	if strings.Contains(string(raw), "09:00:00") {
		t.Error("the oldest keys should have fallen off the front")
	}
}

// A launch amenbo named no project for still has somewhere to remember: a corner of its own, kept
// apart from every project rather than shared with them. A run by hand is what lands there.
func TestARunWithNoProjectNamedKeepsItsOwnCorner(t *testing.T) {
	home := t.TempDir()
	t.Setenv(storeEnv, home)
	t.Setenv(reachEnv, "")
	slackStands(t)
	readsBack(t, "AMB-T-42", "Ship the thing")

	if err := hook(moment(eventTaskDone, "2026-07-22T09:00:00Z")); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(stateDir(home, noReach), takenFile)
	if _, err := os.Stat(path); err != nil {
		t.Errorf("nothing was written down at %s: %v", path, err)
	}
}

// The project arrives as an environment variable's value, and the value names a directory. So what
// it can name is one directory under the plugin, whatever it says — a reach that read as a path
// would write the plugin's state outside the plugin.
func TestAReachThatWouldWalkOutStaysUnderThePlugin(t *testing.T) {
	home := t.TempDir()
	t.Setenv(storeEnv, home)
	t.Setenv(reachEnv, "../../elsewhere")
	slackStands(t)
	readsBack(t, "AMB-T-42", "Ship the thing")

	if err := hook(moment(eventTaskDone, "2026-07-22T09:00:00Z")); err != nil {
		t.Fatal(err)
	}

	dir := reachDir()
	if strings.ContainsAny(dir, `/\`) || strings.Contains(dir, "..") {
		t.Fatalf("a reach should name one directory and nothing above it, got %q", dir)
	}
	path := filepath.Join(stateDir(home, dir), takenFile)
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the record should still be under the plugin, at %s: %v", path, err)
	}
}

// State from before it was kept per project is dropped rather than carried over: it was written
// while every project shared it, so no project can claim it. Here the shared record holds this very
// event's key — adopting it would swallow the message — and the shared batch holds a line from
// somewhere unknown.
func TestStateFromBeforeTheProjectSplitIsDropped(t *testing.T) {
	home := remembers(t)
	posted := slackStands(t)
	readsBack(t, "AMB-T-42", "Ship the thing")

	shared := filepath.Join(home, "plugins", pluginName)
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	left := map[string]string{
		pendingFile: "\"AI created AMB-T-9 — from some project or other\"\n",
		takenFile:   "task.done 42 2026-07-22T09:00:00Z\n",
	}
	for name, body := range left {
		if err := os.WriteFile(filepath.Join(shared, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	behind(t, 0)
	if err := hook(moment(eventTaskDone, "2026-07-22T09:00:00Z")); err != nil {
		t.Fatal(err)
	}

	if len(*posted) != 1 {
		t.Fatalf("this project's own event should go out, got %v", *posted)
	}
	if strings.Contains((*posted)[0], "from some project or other") {
		t.Errorf("a line no project can claim should not go out here: %q", (*posted)[0])
	}
	for name := range left {
		if _, err := os.Stat(filepath.Join(shared, name)); !os.IsNotExist(err) {
			t.Errorf("%s should have been taken away: %v", name, err)
		}
	}
}

// A run with nowhere to write reports anyway: a duplicate is a smaller fault than a silence, and a
// hand run outside amenbo has no store named to it at all.
func TestHookReportsWithNoStoreToRememberIn(t *testing.T) {
	t.Setenv(storeEnv, "")
	posted := slackStands(t)
	readsBack(t, "AMB-T-42", "Ship the thing")

	if err := hook(moment(eventTaskDone, "2026-07-22T09:00:00Z")); err != nil {
		t.Fatal(err)
	}

	if len(*posted) != 1 {
		t.Errorf("the message should still go out, got %v", *posted)
	}
}

// A store that cannot be written down to is a message that may be sent again, and one that was never
// held back — which is worth a failed run, since that is where the reason becomes visible. What it is
// not worth is a silence.
func TestHookFailsWhenItCannotWriteDownWhatItSent(t *testing.T) {
	// A store whose path is a file has no `plugins/` under it to make.
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(storeEnv, blocked)
	t.Setenv(reachQueueRemainingEnv, "5")
	posted := slackStands(t)
	readsBack(t, "AMB-T-42", "Ship the thing")

	err := hook(moment(eventTaskDone, "2026-07-22T09:00:00Z"))

	if len(*posted) != 1 {
		t.Fatalf("the message should go out even with a queue behind it, got %v", *posted)
	}
	if err == nil {
		t.Error("the run should end on the store it could not write")
	}
}
