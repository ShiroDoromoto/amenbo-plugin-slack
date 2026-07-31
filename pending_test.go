package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// behind says how many events the runner has left after this launch.
func behind(t *testing.T, count int) {
	t.Helper()
	t.Setenv(queueRemainingEnv, strconv.Itoa(count))
}

// A burst is one act to the user, so it arrives as one message: every launch with something behind it
// holds its line, and the launch that sees nothing behind it sends them all in order.
func TestHookHoldsALineWhileTheQueueIsBehindIt(t *testing.T) {
	remembers(t)
	posted := slackStands(t)
	readsBack(t, "AMB-T-42", "Ship the thing")

	behind(t, 2)
	if err := hook(moment(eventTaskCreated, "2026-07-22T09:00:00Z")); err != nil {
		t.Fatal(err)
	}
	behind(t, 1)
	moved := moment(eventStatusChanged, "2026-07-22T09:00:01Z")
	moved.New = "in_progress"
	if err := hook(moved); err != nil {
		t.Fatal(err)
	}
	if len(*posted) != 0 {
		t.Fatalf("nothing should go out while the queue is behind it, got %v", *posted)
	}

	behind(t, 0)
	if err := hook(moment(eventTaskDone, "2026-07-22T09:00:02Z")); err != nil {
		t.Fatal(err)
	}

	if len(*posted) != 1 {
		t.Fatalf("the burst should arrive as one message, got %v", *posted)
	}
	want := strings.Join([]string{
		"AI created AMB-T-42 — Ship the thing",
		"AI moved AMB-T-42 to In progress — Ship the thing",
		"AI finished AMB-T-42 — Ship the thing",
	}, "\n")
	if (*posted)[0] != want {
		t.Errorf("the lines should be in the order they happened:\n%q", (*posted)[0])
	}
}

// channel stands in for one project's Slack channel: it collects what is posted to it, and hands back
// the webhook a run reaching that project would be given.
func channel(t *testing.T) (*[]string, string) {
	t.Helper()
	var posted []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Text string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("the message should be JSON: %v", err)
		}
		posted = append(posted, body.Text)
		fmt.Fprint(w, "ok")
	}))
	t.Cleanup(server.Close)
	return &posted, server.URL
}

// launch is one event delivered for one project: the reach and the webhook amenbo hands over are that
// project's, and both change from launch to launch on a store holding two.
func launch(t *testing.T, project, webhook string, queued int, in input) {
	t.Helper()
	t.Setenv(reachEnv, project)
	t.Setenv(webhookEnv, webhook)
	behind(t, queued)
	if err := hook(in); err != nil {
		t.Fatal(err)
	}
}

// A store holds every project at once while a webhook belongs to one, so what is held back is held
// back for the project it came from: another project's launch flushes its own lines and only its own,
// or a burst in one project would be posted into another's channel.
func TestALineHeldForOneProjectIsNotFlushedByAnother(t *testing.T) {
	t.Setenv(storeEnv, t.TempDir())
	readsBack(t, "AMB-T-42", "Ship the thing")
	first, firstHook := channel(t)
	second, secondHook := channel(t)

	// One project's burst: its line waits for the rest of that project's queue.
	launch(t, "AMB-P-1", firstHook, 1, moment(eventTaskCreated, "2026-07-22T09:00:00Z"))
	// The other project's launch has nothing behind it — and nothing of the first project's to send.
	launch(t, "AMB-P-2", secondHook, 0, moment(eventTaskDone, "2026-07-22T09:00:01Z"))

	if len(*second) != 1 || (*second)[0] != "AI finished AMB-T-42 — Ship the thing" {
		t.Errorf("a channel should carry its own project's lines alone, got %v", *second)
	}
	if len(*first) != 0 {
		t.Fatalf("the first project's line should still be waiting, got %v", *first)
	}

	// It goes out when its own queue empties, late rather than lost.
	launch(t, "AMB-P-1", firstHook, 0, moment(eventTaskDone, "2026-07-22T09:00:02Z"))

	if len(*first) != 1 || !strings.Contains((*first)[0], "AI created AMB-T-42") {
		t.Errorf("the held line should arrive on its own project's flush, got %v", *first)
	}
}

// What is held is written down before anything else happens, because a launch ends after one event and
// the row it was for has already left the queue — nothing on amenbo's side is waiting to hand it over
// again.
func TestWhatIsHeldSurvivesTheRunThatHeldIt(t *testing.T) {
	home := remembers(t)
	slackStands(t)
	readsBack(t, "AMB-T-42", "Ship the thing")

	behind(t, 1)
	if err := hook(moment(eventTaskCreated, "2026-07-22T09:00:00Z")); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(stateDir(home, oneProject), pendingFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("what is owed should be on disk at %s: %v", path, err)
	}
	if !strings.Contains(string(raw), "AI created AMB-T-42") {
		t.Errorf("unexpected contents: %q", raw)
	}
}

// Once the batch has gone out, nothing is owed — the next burst starts empty rather than repeating
// what a channel already has.
func TestWhatWentOutIsNoLongerOwed(t *testing.T) {
	home := remembers(t)
	posted := slackStands(t)
	readsBack(t, "AMB-T-42", "Ship the thing")

	behind(t, 1)
	if err := hook(moment(eventTaskCreated, "2026-07-22T09:00:00Z")); err != nil {
		t.Fatal(err)
	}
	behind(t, 0)
	if err := hook(moment(eventTaskDone, "2026-07-22T09:00:01Z")); err != nil {
		t.Fatal(err)
	}
	if err := hook(moment(eventTaskDone, "2026-07-22T09:00:02Z")); err != nil {
		t.Fatal(err)
	}

	if len(*posted) != 2 {
		t.Fatalf("two flushes are two messages, got %v", *posted)
	}
	if strings.Contains((*posted)[1], "AI created") {
		t.Errorf("the second message repeats what the first carried: %q", (*posted)[1])
	}
	if _, err := os.Stat(filepath.Join(stateDir(home, oneProject), pendingFile)); !os.IsNotExist(err) {
		t.Errorf("nothing should be owed after a flush: %v", err)
	}
}

// A flush that Slack refused leaves everything still owed, so the next one carries it. amenbo does not
// retry the event, and this is what stands in for that: the lines are not lost, they are late.
func TestAFlushThatFailedLeavesTheLinesOwed(t *testing.T) {
	remembers(t)
	refuse := true
	var posted []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if refuse {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		body := make([]byte, r.ContentLength)
		if _, err := r.Body.Read(body); err != nil && len(body) == 0 {
			t.Errorf("reading the message: %v", err)
		}
		posted = append(posted, string(body))
	}))
	t.Cleanup(server.Close)
	t.Setenv(webhookEnv, server.URL)
	readsBack(t, "AMB-T-42", "Ship the thing")

	behind(t, 0)
	if err := hook(moment(eventTaskCreated, "2026-07-22T09:00:00Z")); err == nil {
		t.Fatal("a webhook that refuses should fail the run")
	}
	refuse = false
	if err := hook(moment(eventTaskDone, "2026-07-22T09:00:01Z")); err != nil {
		t.Fatal(err)
	}

	if len(posted) != 1 {
		t.Fatalf("the next flush should carry both, got %v", posted)
	}
	for _, want := range []string{"AI created AMB-T-42", "AI finished AMB-T-42"} {
		if !strings.Contains(posted[0], want) {
			t.Errorf("the refused line should still arrive: %q", posted[0])
		}
	}
}

// The flush is the runner's question, not the event's. A burst that ends in events nobody asked to
// hear about still empties what is waiting — otherwise the lines in front of them would sit there
// until the next reportable write, which may be hours away or never.
func TestAnEventNobodyAskedForStillFlushesWhatIsWaiting(t *testing.T) {
	remembers(t)
	posted := slackStands(t)
	readsBack(t, "AMB-T-42", "Ship the thing")

	behind(t, 1)
	if err := hook(moment(eventTaskCreated, "2026-07-22T09:00:00Z")); err != nil {
		t.Fatal(err)
	}
	if len(*posted) != 0 {
		t.Fatalf("nothing should go out yet, got %v", *posted)
	}

	// Last in the pass, and outside the default four: nothing of its own to say.
	behind(t, 0)
	if err := hook(moment(eventTaskDeleted, "2026-07-22T09:00:01Z")); err != nil {
		t.Fatal(err)
	}

	if len(*posted) != 1 || *posted == nil {
		t.Fatalf("what was waiting should have gone out, got %v", *posted)
	}
	if (*posted)[0] != "AI created AMB-T-42 — Ship the thing" {
		t.Errorf("the unasked-for event should add no line of its own: %q", (*posted)[0])
	}
}

// A second delivery adds no line, and does not stop the flush either: it may be the launch with
// nothing behind it.
func TestAReplayStillFlushesWhatIsWaiting(t *testing.T) {
	remembers(t)
	posted := slackStands(t)
	readsBack(t, "AMB-T-42", "Ship the thing")

	behind(t, 1)
	created := moment(eventTaskCreated, "2026-07-22T09:00:00Z")
	if err := hook(created); err != nil {
		t.Fatal(err)
	}
	behind(t, 0)
	if err := hook(created); err != nil {
		t.Fatal(err)
	}

	if len(*posted) != 1 || (*posted)[0] != "AI created AMB-T-42 — Ship the thing" {
		t.Errorf("one line, sent once, got %v", *posted)
	}
}

// An amenbo that says nothing about its queue, and a run by hand, both mean "nothing is behind this" —
// one message per event, the way it worked before the runner counted.
func TestHookSendsAtOnceWhenNothingSaysWhatIsBehind(t *testing.T) {
	remembers(t)
	posted := slackStands(t)
	readsBack(t, "AMB-T-42", "Ship the thing")

	t.Setenv(queueRemainingEnv, "")
	if err := hook(moment(eventTaskDone, "2026-07-22T09:00:00Z")); err != nil {
		t.Fatal(err)
	}
	t.Setenv(queueRemainingEnv, "not a number")
	if err := hook(moment(eventTaskDone, "2026-07-22T09:00:01Z")); err != nil {
		t.Fatal(err)
	}

	if len(*posted) != 2 {
		t.Errorf("each event should be its own message, got %v", *posted)
	}
}

// With nowhere to keep what is owed, nothing is held back: a message waiting in a process that is about
// to end is a message nobody ever gets.
func TestHookHoldsNothingBackWithNoStoreToHoldItIn(t *testing.T) {
	t.Setenv(storeEnv, "")
	posted := slackStands(t)
	readsBack(t, "AMB-T-42", "Ship the thing")

	behind(t, 3)
	if err := hook(moment(eventTaskDone, "2026-07-22T09:00:00Z")); err != nil {
		t.Fatal(err)
	}

	if len(*posted) != 1 {
		t.Errorf("the message should go out at once, got %v", *posted)
	}
}

// A title is the user's text, and a message with a newline in it must not arrive as two.
func TestAHeldMessageSurvivesANewlineInATitle(t *testing.T) {
	remembers(t)
	posted := slackStands(t)
	readsBack(t, "AMB-T-42", "two\nlines")

	behind(t, 1)
	if err := hook(moment(eventTaskCreated, "2026-07-22T09:00:00Z")); err != nil {
		t.Fatal(err)
	}
	behind(t, 0)
	if err := hook(moment(eventTaskDone, "2026-07-22T09:00:01Z")); err != nil {
		t.Fatal(err)
	}

	if len(*posted) != 1 {
		t.Fatalf("one flush is one message, got %v", *posted)
	}
	if lines := strings.Count((*posted)[0], "\n"); lines != 3 {
		t.Errorf("two messages of two lines each is three breaks, got %d: %q", lines, (*posted)[0])
	}
}
