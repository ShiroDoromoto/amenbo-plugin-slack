package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// remembers points the plugin at a store of its own for the length of one test, so what it writes
// down lands in a temp directory rather than in anyone's app-data.
func remembers(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv(storeEnv, home)
	return home
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

// The record lives in the plugin's own installed directory, so removing the plugin takes it away
// with everything else it left behind.
func TestTheRecordLivesWithTheInstalledPlugin(t *testing.T) {
	home := remembers(t)
	slackStands(t)
	readsBack(t, "AMB-T-42", "Ship the thing")

	if err := hook(moment(eventTaskDone, "2026-07-22T09:00:00Z")); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(home, "plugins", pluginName, takenFile)
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

	raw, err := os.ReadFile(filepath.Join(home, "plugins", pluginName, takenFile))
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
	t.Setenv(queueRemainingEnv, "5")
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
