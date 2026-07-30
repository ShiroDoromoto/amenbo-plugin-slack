package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// aiWrite is the payload for one write the user's AI drove.
func aiWrite(event string) input {
	return input{V: contractVersion, Event: event, ID: 42, Actor: actorAI}
}

// slackStands stands in for Slack: it collects every message posted to it and hands back the
// webhook URL to post to, which the test puts where amenbo would have.
func slackStands(t *testing.T) *[]string {
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
	t.Setenv(webhookEnv, server.URL)
	return &posted
}

// readsBack stands in for amenbo, answering `task show --json` with one task.
func readsBack(t *testing.T, ref, title string) {
	t.Helper()
	previous := runAmenbo
	runAmenbo = func(args ...string) ([]byte, error) {
		if strings.Join(args, " ") != "task show 42 --json --actor ai" {
			t.Errorf("the title should be read back by the id the payload carried, got %v", args)
		}
		return []byte(fmt.Sprintf(`{"ref":%q,"title":%q,"status":"done"}`, ref, title)), nil
	}
	t.Cleanup(func() { runAmenbo = previous })
}

// refusesToRead stands in for an amenbo that would not answer.
func refusesToRead(t *testing.T, because string) {
	t.Helper()
	previous := runAmenbo
	runAmenbo = func(args ...string) ([]byte, error) { return nil, fmt.Errorf("%s", because) }
	t.Cleanup(func() { runAmenbo = previous })
}

// One event the AI drove is one message, and it says what was done to which task.
func TestHookSendsOneMessagePerEvent(t *testing.T) {
	posted := slackStands(t)
	readsBack(t, "AMB-T-42", "Ship the thing")

	if err := hook(aiWrite(eventTaskDone)); err != nil {
		t.Fatal(err)
	}

	if len(*posted) != 1 {
		t.Fatalf("one event is one message, got %d: %v", len(*posted), *posted)
	}
	if (*posted)[0] != "AI finished AMB-T-42 — Ship the thing" {
		t.Errorf("unexpected message: %q", (*posted)[0])
	}
}

// Each reported event has its own sentence, and a status change carries the state it moved to.
func TestSentenceSaysWhatHappened(t *testing.T) {
	for _, c := range []struct {
		event, newState, want string
	}{
		{eventTaskCreated, "", "AI created AMB-T-42 — Ship the thing"},
		{eventTaskDone, "", "AI finished AMB-T-42 — Ship the thing"},
		{eventTaskRejected, "", "AI decided against AMB-T-42 — Ship the thing"},
		{eventStatusChanged, "in_progress", "AI moved AMB-T-42 to in_progress — Ship the thing"},
	} {
		if got := sentence(c.event, c.newState, "AMB-T-42", "Ship the thing"); got != c.want {
			t.Errorf("%s: got %q, want %q", c.event, got, c.want)
		}
	}
}

// The user's own writes are not reported: they were there for them.
func TestHookIsSilentAboutWhatTheUserDid(t *testing.T) {
	posted := slackStands(t)
	readsBack(t, "AMB-T-42", "Ship the thing")

	human := aiWrite(eventTaskDone)
	human.Actor = "human"
	if err := hook(human); err != nil {
		t.Fatal(err)
	}

	if len(*posted) != 0 {
		t.Errorf("nothing should be posted, got %v", *posted)
	}
}

// An event outside the four, and a contract this plugin cannot read, are silences too — runs
// with nothing to say, not runs that went wrong.
func TestHookIsSilentOnWhatItDoesNotReport(t *testing.T) {
	posted := slackStands(t)
	readsBack(t, "AMB-T-42", "Ship the thing")

	unreported := aiWrite("comment.added")
	otherContract := aiWrite(eventTaskDone)
	otherContract.V = contractVersion + 1

	for _, in := range []input{unreported, otherContract} {
		if err := hook(in); err != nil {
			t.Errorf("%+v: silence is not a failure: %v", in, err)
		}
	}
	if len(*posted) != 0 {
		t.Errorf("nothing should be posted, got %v", *posted)
	}
}

// A title that could not be read back costs the message its title, not the message. The run
// still ends non-zero, so the fault is in the execution log rather than in every message from
// here on.
func TestHookReportsWithoutATitleItCouldNotRead(t *testing.T) {
	posted := slackStands(t)
	refusesToRead(t, "out_of_reach")

	err := hook(aiWrite(eventTaskDone))

	if len(*posted) != 1 || (*posted)[0] != "AI finished task #42" {
		t.Fatalf("the message should still go out, naming the task by its number: %v", *posted)
	}
	if err == nil || !strings.Contains(err.Error(), "out_of_reach") {
		t.Errorf("the run should end on the read that failed, got %v", err)
	}
}

// A webhook that refuses the message is a failure with nothing reported, and what it refused
// it with is what the log needs.
func TestHookFailsWhenTheWebhookRefuses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "no_service")
	}))
	t.Cleanup(server.Close)
	t.Setenv(webhookEnv, server.URL)
	readsBack(t, "AMB-T-42", "Ship the thing")

	err := hook(aiWrite(eventTaskDone))

	if err == nil || !strings.Contains(err.Error(), "no_service") {
		t.Errorf("the refusal should be carried back: %v", err)
	}
}

// The setting is required, so an empty one means it was taken away from under an open gate —
// worth saying, and worth saying how to put it back.
func TestHookFailsWithoutAWebhook(t *testing.T) {
	t.Setenv(webhookEnv, "")

	err := hook(aiWrite(eventTaskDone))

	if err == nil || !strings.Contains(err.Error(), "webhook_url") {
		t.Errorf("the failure should name the setting to fill in, got %v", err)
	}
}
