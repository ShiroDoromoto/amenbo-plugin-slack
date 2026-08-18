package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// aiWrite is the payload for one write the user's AI drove, with the setting unanswered — so what
// it reports is the default four.
func aiWrite(event string) input {
	return input{V: contractVersion, Event: event, ID: 42, Actor: actorAI}
}

// reporting is the same payload with the setting answered, naming the events to report.
func reporting(event string, chosen ...string) input {
	in := aiWrite(event)
	in.Config = map[string]any{eventsSetting: strings.Join(chosen, ",")}
	return in
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

// englishStore is a store whose user reads in English and left their AI named as it comes. Every
// message expected below is worded from it, so the English in them is the store's answer rather
// than something this plugin decided.
const englishStore = `{"settings":{"language":"en","ai_display_name":"AI"}}`

// readsBack stands in for amenbo, answering a `show --json` on any record with one ref and title,
// and the settings with the English store above.
func readsBack(t *testing.T, ref, title string) *[]string {
	t.Helper()
	var asked []string
	previous := runAmenbo
	runAmenbo = func(args ...string) ([]byte, error) {
		asked = append(asked, strings.Join(args, " "))
		if len(args) > 0 && args[0] == "config" {
			return []byte(englishStore), nil
		}
		return []byte(fmt.Sprintf(`{"ref":%q,"title":%q,"status":"done"}`, ref, title)), nil
	}
	t.Cleanup(func() { runAmenbo = previous })
	return &asked
}

// namesTheProject stands in for an amenbo that answers `project show` with a name, and every other
// read the way readsBack does.
func namesTheProject(t *testing.T, name string) *[]string {
	t.Helper()
	var asked []string
	previous := runAmenbo
	runAmenbo = func(args ...string) ([]byte, error) {
		asked = append(asked, strings.Join(args, " "))
		switch {
		case len(args) > 0 && args[0] == "project":
			return []byte(fmt.Sprintf(`{"name":%q}`, name)), nil
		case len(args) > 0 && args[0] == "config":
			return []byte(englishStore), nil
		}
		return []byte(`{"ref":"AMB-T-42","title":"Ship the thing"}`), nil
	}
	t.Cleanup(func() { runAmenbo = previous })
	return &asked
}

// records is the reads a message's *content* came from — the records it names. The settings behind
// its wording are read on every line and are their own tests' business, so they are left out here.
func records(asked *[]string) []string {
	var kept []string
	for _, one := range *asked {
		if !strings.HasPrefix(one, "config ") {
			kept = append(kept, one)
		}
	}
	return kept
}

// refusesToRead stands in for an amenbo that would not answer.
func refusesToRead(t *testing.T, because string) {
	t.Helper()
	previous := runAmenbo
	runAmenbo = func(args ...string) ([]byte, error) { return nil, fmt.Errorf("%s", because) }
	t.Cleanup(func() { runAmenbo = previous })
}

// One event the AI drove is one message, and it says what was done to which task — read back by the
// id the payload carried, under a declared facet.
func TestHookSendsOneMessagePerEvent(t *testing.T) {
	posted := slackStands(t)
	asked := readsBack(t, "AMB-T-42", "Ship the thing")

	if err := hook(aiWrite(eventTaskDone)); err != nil {
		t.Fatal(err)
	}

	if len(*posted) != 1 {
		t.Fatalf("one event is one message, got %d: %v", len(*posted), *posted)
	}
	if (*posted)[0] != "AI finished AMB-T-42 — Ship the thing" {
		t.Errorf("unexpected message: %q", (*posted)[0])
	}
	if read := records(asked); len(read) != 1 || read[0] != "task show 42 --json --actor ai" {
		t.Errorf("unexpected read back: %v", read)
	}
}

// readsBackInJapanese is an amenbo whose user reads in Japanese and named their AI.
func readsBackInJapanese(t *testing.T) *[]string {
	t.Helper()
	var asked []string
	previous := runAmenbo
	runAmenbo = func(args ...string) ([]byte, error) {
		asked = append(asked, strings.Join(args, " "))
		if len(args) > 0 && args[0] == "config" {
			return []byte(`{"settings":{"language":"ja","ai_display_name":"さくら"}}`), nil
		}
		return []byte(`{"ref":"AMB-T-42","title":"Ship the thing"}`), nil
	}
	t.Cleanup(func() { runAmenbo = previous })
	return &asked
}

// A message is worded in the language the store is read in, and its subject is the name the user
// gave their AI. The title stays as it was written — it is theirs, not this plugin's to translate.
func TestAMessageIsWordedTheWayTheStoreIsRead(t *testing.T) {
	posted := slackStands(t)
	asked := readsBackInJapanese(t)

	if err := hook(aiWrite(eventTaskDone)); err != nil {
		t.Fatal(err)
	}

	if len(*posted) != 1 || (*posted)[0] != "さくら が AMB-T-42 を完了しました — Ship the thing" {
		t.Fatalf("unexpected message: %v", *posted)
	}
	if read := strings.Join(*asked, "\n"); !strings.Contains(read, "config --json --actor ai") {
		t.Errorf("the wording should be read back under a declared facet: %q", read)
	}
}

// The settings are read where a line is worded, and only there: a launch that has nothing to add —
// one whose event was already taken in — words nothing, so it asks nothing, even when it is the
// launch that sends.
func TestTheWordingIsReadWhereALineIsWorded(t *testing.T) {
	remembers(t)
	slackStands(t)
	asked := readsBackInJapanese(t)

	behind(t, 1)
	twice := moment(eventTaskDone, "2026-07-22T09:00:00Z")
	if err := hook(twice); err != nil {
		t.Fatal(err)
	}
	behind(t, 0)
	if err := hook(twice); err != nil {
		t.Fatal(err)
	}

	reads := 0
	for _, args := range *asked {
		if strings.HasPrefix(args, "config ") {
			reads++
		}
	}
	if reads != 1 {
		t.Errorf("one line worded is one read, got %d: %v", reads, *asked)
	}
}

// A deleted task cannot be read back, so its title rides on the vanished record the payload carries
// in place of the record itself.
func TestHookNamesADeletedTaskFromTheVanishedRecord(t *testing.T) {
	posted := slackStands(t)
	asked := readsBack(t, "AMB-T-42", "read back, which should not happen here")

	in := reporting(eventTaskDeleted, eventTaskDeleted)
	in.Record = map[string]any{"id": float64(42), "title": "Ship the thing", "status": "todo"}
	if err := hook(in); err != nil {
		t.Fatal(err)
	}

	if len(*posted) != 1 || (*posted)[0] != "AI deleted task #42 — Ship the thing" {
		t.Fatalf("unexpected message: %v", *posted)
	}
	if read := records(asked); len(read) != 0 {
		t.Errorf("there is nothing left to read back: %v", read)
	}
}

// A comment taken back names the task it hung on, which the payload carries as the parent — so that
// one is read back after all, by the parent's id rather than the comment's.
func TestHookNamesTheTaskARemovedCommentHungOn(t *testing.T) {
	posted := slackStands(t)
	asked := readsBack(t, "AMB-T-7", "Ship the thing")

	parent := int64(7)
	in := reporting(eventCommentRemoved, eventCommentRemoved)
	in.Parent = &parent
	if err := hook(in); err != nil {
		t.Fatal(err)
	}

	if len(*posted) != 1 || (*posted)[0] != "AI took back a comment on AMB-T-7 — Ship the thing" {
		t.Fatalf("unexpected message: %v", *posted)
	}
	if read := records(asked); len(read) != 1 || !strings.HasPrefix(read[0], "task show 7 ") {
		t.Errorf("the parent is what gets read: %v", read)
	}
}

// A comment added names its task too, and by the same road: the parent is on the wire for both
// comment events, and a comment's own number is not what a reader in a channel can follow.
func TestHookNamesTheTaskAnAddedCommentHangsOn(t *testing.T) {
	posted := slackStands(t)
	asked := readsBack(t, "AMB-T-7", "Ship the thing")

	parent := int64(7)
	in := reporting(eventCommentAdded, eventCommentAdded)
	in.Parent = &parent
	if err := hook(in); err != nil {
		t.Fatal(err)
	}

	if len(*posted) != 1 || (*posted)[0] != "AI added a comment on AMB-T-7 — Ship the thing" {
		t.Fatalf("unexpected message: %v", *posted)
	}
	if read := records(asked); len(read) != 1 || !strings.HasPrefix(read[0], "task show 7 ") {
		t.Errorf("the parent is what gets read: %v", read)
	}
}

// An amenbo from before the parent was on the wire sends the event without one. The field was
// added rather than changed, so nothing is refused: the message names the comment by its own
// number, which is all that arrived.
func TestHookFallsBackToTheNumberWhenNoParentArrives(t *testing.T) {
	posted := slackStands(t)
	asked := readsBack(t, "AMB-T-42", "Ship the thing")

	if err := hook(reporting(eventCommentAdded, eventCommentAdded)); err != nil {
		t.Fatal(err)
	}

	if len(*posted) != 1 || (*posted)[0] != "AI added comment #42" {
		t.Fatalf("unexpected message: %v", *posted)
	}
	if read := records(asked); len(read) != 0 {
		t.Errorf("there is nothing to read a comment back with: %v", read)
	}
}

// A decision is read back on its own axis — the same two fields, a different record.
func TestHookReadsADecisionBackAsADecision(t *testing.T) {
	posted := slackStands(t)
	asked := readsBack(t, "AMB-D-42", "Report only the AI's writes")

	if err := hook(reporting(eventDecisionAccepted, eventDecisionAccepted)); err != nil {
		t.Fatal(err)
	}

	if len(*posted) != 1 || (*posted)[0] != "AI accepted AMB-D-42 — Report only the AI's writes" {
		t.Fatalf("unexpected message: %v", *posted)
	}
	if read := records(asked); len(read) != 1 || !strings.HasPrefix(read[0], "decision show 42 ") {
		t.Errorf("a decision is not read back as a task: %v", read)
	}
}

// The choice is what decides: an event the user asked for is reported, and one they did not is not,
// however plainly it was fired.
func TestHookReportsWhatTheUserChose(t *testing.T) {
	posted := slackStands(t)
	readsBack(t, "AMB-T-42", "Ship the thing")

	if err := hook(reporting(eventCommentAdded, eventCommentAdded, eventTaskMoved)); err != nil {
		t.Fatal(err)
	}
	if err := hook(reporting(eventTaskDone, eventCommentAdded, eventTaskMoved)); err != nil {
		t.Fatal(err)
	}

	if len(*posted) != 1 || (*posted)[0] != "AI added comment #42" {
		t.Errorf("only the chosen event should be reported: %v", *posted)
	}
}

// Choosing none is an answer, and it is honoured: an empty setting reports nothing at all, rather
// than being read as unset and quietly replaced by the default.
func TestHookIsSilentWhenTheUserChoseNone(t *testing.T) {
	posted := slackStands(t)
	readsBack(t, "AMB-T-42", "Ship the thing")

	for _, event := range catalog {
		if err := hook(reporting(event)); err != nil {
			t.Errorf("%s: silence is not a failure: %v", event, err)
		}
	}

	if len(*posted) != 0 {
		t.Errorf("nothing should be posted, got %v", *posted)
	}
}

// No setting at all is not an answer — an amenbo from before it, or a manifest without it — so the
// four the manifest declares as its default are what such a build reports.
func TestHookFallsBackToTheDefaultWhenNothingWasDeclared(t *testing.T) {
	posted := slackStands(t)
	readsBack(t, "AMB-T-42", "Ship the thing")

	for _, event := range catalog {
		if err := hook(aiWrite(event)); err != nil {
			t.Errorf("%s: %v", event, err)
		}
	}

	if len(*posted) != len(defaultEvents) {
		t.Errorf("the default four should be reported, got %v", *posted)
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

// A contract this plugin cannot read is a silence too — a run with nothing to say, not one that went
// wrong.
func TestHookIsSilentOnAContractItDoesNotRead(t *testing.T) {
	posted := slackStands(t)
	readsBack(t, "AMB-T-42", "Ship the thing")

	otherContract := aiWrite(eventTaskDone)
	otherContract.V = contractVersion + 1
	if err := hook(otherContract); err != nil {
		t.Errorf("silence is not a failure: %v", err)
	}

	if len(*posted) != 0 {
		t.Errorf("nothing should be posted, got %v", *posted)
	}
}

// A title that could not be read back costs the message its title, not the message. The run still
// ends non-zero, so the fault is in the execution log rather than in every message from here on.
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

// A webhook that refuses the message is a failure with nothing reported, and what it refused it with
// is what the log needs.
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

// The setting is required, so an empty one means it was taken away from under an open gate — worth
// saying, and worth saying how to put it back.
func TestHookFailsWithoutAWebhook(t *testing.T) {
	t.Setenv(webhookEnv, "")

	err := hook(aiWrite(eventTaskDone))

	if err == nil || !strings.Contains(err.Error(), "webhook_url") {
		t.Errorf("the failure should name the setting to fill in, got %v", err)
	}
}

// A message says which project it is about, once, at the top. The channel used to be that answer,
// and stops being one as soon as two projects report into the same room.
func TestAMessageLeadsWithTheProjectItCameFrom(t *testing.T) {
	posted := slackStands(t)
	asked := namesTheProject(t, "Ship the thing, the project")
	t.Setenv(reachEnv, "AMB-P-9")

	if err := hook(aiWrite(eventTaskDone)); err != nil {
		t.Fatal(err)
	}

	want := "*Ship the thing, the project*\nAI finished AMB-T-42 — Ship the thing"
	if len(*posted) != 1 || (*posted)[0] != want {
		t.Fatalf("the message should lead with the project, got %v", *posted)
	}
	if read := strings.Join(*asked, "\n"); !strings.Contains(read, "project show AMB-P-9 --json --actor ai") {
		t.Errorf("the project should be read back by the ref amenbo handed over: %q", read)
	}
}

// The heading belongs to the message, not to the line: a batch is read for once, on the run that
// sends it, rather than once per event taken in.
func TestTheProjectIsReadOnceForAWholeBatch(t *testing.T) {
	remembers(t)
	posted := slackStands(t)
	asked := namesTheProject(t, "Work")

	behind(t, 2)
	if err := hook(moment(eventTaskCreated, "2026-07-22T09:00:00Z")); err != nil {
		t.Fatal(err)
	}
	behind(t, 1)
	if err := hook(moment(eventTaskDone, "2026-07-22T09:00:01Z")); err != nil {
		t.Fatal(err)
	}
	behind(t, 0)
	if err := hook(moment(eventTaskRejected, "2026-07-22T09:00:02Z")); err != nil {
		t.Fatal(err)
	}

	if len(*posted) != 1 {
		t.Fatalf("the burst should arrive as one message, got %v", *posted)
	}
	if headings := strings.Count((*posted)[0], "*Work*"); headings != 1 {
		t.Errorf("one message carries one heading, got %d: %q", headings, (*posted)[0])
	}
	reads := 0
	for _, args := range *asked {
		if strings.HasPrefix(args, "project show") {
			reads++
		}
	}
	if reads != 1 {
		t.Errorf("the project should be read on the flush alone, got %d reads", reads)
	}
}

// A project that could not be read costs the message its heading and nothing else — the same trade
// as a title that could not be read. The run ends non-zero so the reason reaches the execution log.
func TestAMessageGoesOutWithoutAHeadingItCouldNotRead(t *testing.T) {
	posted := slackStands(t)
	t.Setenv(reachEnv, "AMB-P-9")
	previous := runAmenbo
	runAmenbo = func(args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "project" {
			return nil, fmt.Errorf("out_of_reach")
		}
		return []byte(`{"ref":"AMB-T-42","title":"Ship the thing"}`), nil
	}
	t.Cleanup(func() { runAmenbo = previous })

	err := hook(aiWrite(eventTaskDone))

	if len(*posted) != 1 || (*posted)[0] != "AI finished AMB-T-42 — Ship the thing" {
		t.Fatalf("the message should go out without its heading, got %v", *posted)
	}
	if err == nil || !strings.Contains(err.Error(), "out_of_reach") {
		t.Errorf("the run should end on the read that failed, got %v", err)
	}
}

// A run nothing named a project for — one by hand — has no name to fail to read, so it sends what it
// has and ends cleanly.
func TestARunWithNoProjectNamedSendsNoHeading(t *testing.T) {
	posted := slackStands(t)
	readsBack(t, "AMB-T-42", "Ship the thing")
	t.Setenv(reachEnv, "")

	if err := hook(aiWrite(eventTaskDone)); err != nil {
		t.Fatal(err)
	}

	if len(*posted) != 1 || (*posted)[0] != "AI finished AMB-T-42 — Ship the thing" {
		t.Errorf("nothing should be put at the top, got %v", *posted)
	}
}

// dayCame is the payload for a due date arriving. It is the one event this plugin reports that
// nobody drove, so it names no actor at all — there is no one to have been present for it.
func dayCame(event string, id int64) input {
	return input{V: contractVersion, Event: event, ID: id}
}

// A due date is not a write, so the gate that keeps the user's own writes out of the channel does
// not keep it out either: the day came while nobody was at the desk, which is exactly what a channel
// is for.
func TestHookReportsADueDateNobodyDrove(t *testing.T) {
	posted := slackStands(t)
	readsBack(t, "AMB-T-42", "Ship the thing")

	if err := hook(dayCame(eventTaskDue, 42)); err != nil {
		t.Fatal(err)
	}

	want := "AMB-T-42 is due — Ship the thing"
	if len(*posted) != 1 || (*posted)[0] != want {
		t.Errorf("want %q, got %v", want, *posted)
	}
}

// The other kind says which day it is about, so the two are told apart by the line and not only by
// where it sits.
func TestADueDateTomorrowSaysSo(t *testing.T) {
	posted := slackStands(t)
	readsBack(t, "AMB-T-42", "Ship the thing")

	if err := hook(dayCame(eventTaskDueTomorrow, 42)); err != nil {
		t.Fatal(err)
	}

	want := "AMB-T-42 is due tomorrow — Ship the thing"
	if len(*posted) != 1 || (*posted)[0] != want {
		t.Errorf("want %q, got %v", want, *posted)
	}
}

// A due date the user did not ask to hear about is as silent as any other event: nothing about being
// unattended puts it past the choice.
func TestADueDateIsStillOnlyReportedIfItWasAskedFor(t *testing.T) {
	posted := slackStands(t)
	readsBack(t, "AMB-T-42", "Ship the thing")

	in := dayCame(eventTaskDue, 42)
	in.Config = map[string]any{eventsSetting: eventTaskDone}
	if err := hook(in); err != nil {
		t.Fatal(err)
	}

	if len(*posted) != 0 {
		t.Errorf("an event nobody asked for should be silent, got %v", *posted)
	}
}

// Both kinds are reported until the user says otherwise: a notification nobody opted into is one
// they find out about by missing a deadline first.
func TestADueDateIsReportedUntilTheUserSaysOtherwise(t *testing.T) {
	for _, event := range []string{eventTaskDue, eventTaskDueTomorrow} {
		if !setOf(defaultEvents)[event] {
			t.Errorf("%s should be in what a channel gets by default", event)
		}
	}
}
