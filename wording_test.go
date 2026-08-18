package main

import (
	"strings"
	"testing"
)

// english and japanese are the two stores these tests word a line from.
var (
	english  = preferences{language: "en", aiDisplayName: "AI", humanDisplayName: "Carol"}
	japanese = preferences{language: "ja", aiDisplayName: "さくら", humanDisplayName: "山田"}
)

// lineFor words one line the way hook does, from a payload and what it is about.
func lineFor(how preferences, event, newState string, about subject) string {
	return sentence(how, input{Event: event, New: newState}, about)
}

// Every event in the catalog has its own sentence, and the ones amenbo hands a new state spend it.
// The title trails all of them.
func TestSentenceSaysWhatHappened(t *testing.T) {
	about := subject{name: "AMB-T-42", title: "Ship the thing"}
	for _, c := range []struct {
		event, newState, want string
	}{
		{eventTaskCreated, "", "AI created AMB-T-42 — Ship the thing"},
		{eventTaskDone, "", "AI finished AMB-T-42 — Ship the thing"},
		{eventTaskRejected, "", "AI decided against AMB-T-42 — Ship the thing"},
		{eventStatusChanged, "in_progress", "AI moved AMB-T-42 to In progress — Ship the thing"},
		{eventTaskAssigned, actorHuman, "AI assigned AMB-T-42 to Carol — Ship the thing"},
		{eventTaskMoved, "another-project", "AI moved AMB-T-42 into another-project — Ship the thing"},
		{eventTaskDeleted, "", "AI deleted AMB-T-42 — Ship the thing"},
		{eventDecisionAccepted, "", "AI accepted AMB-T-42 — Ship the thing"},
		{eventDecisionRejected, "", "AI rejected AMB-T-42 — Ship the thing"},
		{eventCommentAdded, "", "AI added AMB-T-42 — Ship the thing"},
		{eventCommentRemoved, "", "AI took back AMB-T-42 — Ship the thing"},
	} {
		if got := lineFor(english, c.event, c.newState, about); got != c.want {
			t.Errorf("%s: got %q, want %q", c.event, got, c.want)
		}
	}
}

// The same eleven in the other language this build carries. A title, a ref and a project's slug
// are the user's own and read the same in both; the sentence around them is what moves.
func TestASentenceIsSaidInTheStoresLanguage(t *testing.T) {
	about := subject{name: "AMB-T-42", title: "Ship the thing"}
	for _, c := range []struct {
		event, newState, want string
	}{
		{eventTaskCreated, "", "さくら が AMB-T-42 を作成しました — Ship the thing"},
		{eventTaskDone, "", "さくら が AMB-T-42 を完了しました — Ship the thing"},
		{eventTaskRejected, "", "さくら が AMB-T-42 をやらないと決めました — Ship the thing"},
		{eventStatusChanged, "in_progress", "さくら が AMB-T-42 を進行中に変更しました — Ship the thing"},
		{eventTaskAssigned, actorHuman, "さくら が AMB-T-42 を 山田 に割り当てました — Ship the thing"},
		{eventTaskMoved, "another-project", "さくら が AMB-T-42 を another-project へ移動しました — Ship the thing"},
		{eventTaskDeleted, "", "さくら が AMB-T-42 を削除しました — Ship the thing"},
		{eventDecisionAccepted, "", "さくら が AMB-T-42 を採択しました — Ship the thing"},
		{eventDecisionRejected, "", "さくら が AMB-T-42 を却下しました — Ship the thing"},
		{eventCommentAdded, "", "さくら が AMB-T-42 を追加しました — Ship the thing"},
		{eventCommentRemoved, "", "さくら が AMB-T-42 を取り消しました — Ship the thing"},
	} {
		if got := lineFor(japanese, c.event, c.newState, about); got != c.want {
			t.Errorf("%s: got %q, want %q", c.event, got, c.want)
		}
	}
}

// A comment is named by the task it hangs on, and that it is a comment is the sentence's to say —
// which is why the two languages put it in different places.
func TestACommentIsSaidAgainstTheTaskItHangsOn(t *testing.T) {
	parent := int64(7)
	about := subject{name: "AMB-T-7", title: "Ship the thing"}
	for _, c := range []struct {
		how   preferences
		event string
		want  string
	}{
		{english, eventCommentAdded, "AI added a comment on AMB-T-7 — Ship the thing"},
		{english, eventCommentRemoved, "AI took back a comment on AMB-T-7 — Ship the thing"},
		{japanese, eventCommentAdded, "さくら が AMB-T-7 にコメントしました — Ship the thing"},
		{japanese, eventCommentRemoved, "さくら が AMB-T-7 のコメントを取り消しました — Ship the thing"},
	} {
		got := sentence(c.how, input{Event: c.event, Parent: &parent}, about)
		if got != c.want {
			t.Errorf("%s in %s: got %q, want %q", c.event, c.how.language, got, c.want)
		}
	}
}

// The events that name a second thing have a form for when it did not arrive.
func TestASentenceHasAFormForWhatDidNotArrive(t *testing.T) {
	about := subject{name: "AMB-T-42"}
	for _, c := range []struct {
		how   preferences
		event string
		want  string
	}{
		{english, eventStatusChanged, "AI moved AMB-T-42"},
		{english, eventTaskMoved, "AI moved AMB-T-42 to another project"},
		{english, eventTaskAssigned, "AI assigned AMB-T-42"},
		{japanese, eventStatusChanged, "さくら が AMB-T-42 の状態を変更しました"},
		{japanese, eventTaskMoved, "さくら が AMB-T-42 を別のプロジェクトへ移動しました"},
		{japanese, eventTaskAssigned, "さくら が AMB-T-42 を割り当てました"},
	} {
		if got := lineFor(c.how, c.event, "", about); got != c.want {
			t.Errorf("%s in %s: got %q, want %q", c.event, c.how.language, got, c.want)
		}
	}
}

// The subject is the name the user gave their AI, not a word baked into the sentence.
func TestTheAIIsTheNameTheUserGaveIt(t *testing.T) {
	about := subject{name: "AMB-T-42"}
	carol := preferences{language: "en", aiDisplayName: "Carol"}

	if got, want := lineFor(carol, eventTaskCreated, "", about), "Carol created AMB-T-42"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A language this build has no wording for is not a message lost: it is said in English, which is
// the row that is always complete. So a language amenbo adds after this build still reports.
func TestALanguageWithNoWordingFallsBackToEnglish(t *testing.T) {
	about := subject{name: "AMB-T-42"}
	unknown := preferences{language: "eo", aiDisplayName: "AI"}

	if got, want := lineFor(unknown, eventTaskCreated, "", about), "AI created AMB-T-42"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := lineFor(unknown, eventStatusChanged, "todo", about), "AI moved AMB-T-42 to To do"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// What a sentence names besides the record arrives as a value off the wire, and each of the three
// leaves differently: a status becomes amenbo's own word, a facet becomes the name that facet goes
// by, and a project's slug stays as it is — it is the store's own value and what the user would go
// and search for.
func TestWhatASentenceNamesBesidesTheRecord(t *testing.T) {
	about := subject{name: "AMB-T-42"}
	for _, c := range []struct {
		how                    preferences
		event, newState, state string
	}{
		{english, eventStatusChanged, "todo", "To do"},
		{english, eventStatusChanged, "blocked", "Blocked"},
		{japanese, eventStatusChanged, "todo", "未着手"},
		{japanese, eventStatusChanged, "blocked", "ブロック"},
		{japanese, eventTaskAssigned, actorAI, "さくら"},
		{japanese, eventTaskAssigned, actorHuman, "山田"},
		{japanese, eventTaskMoved, "another-project", "another-project"},
	} {
		if got := lineFor(c.how, c.event, c.newState, about); !strings.Contains(got, c.state) {
			t.Errorf("%s %q in %s: %q should carry %q", c.event, c.newState, c.how.language, got, c.state)
		}
	}
}

// The same party is said the same way wherever a line names them: the AI that acted and the AI a
// task was handed to are one, so a line that spelled one of them `ai` was calling it two things.
func TestTheOneWhoActedAndTheOneItWentToAreSaidAlike(t *testing.T) {
	about := subject{name: "AMB-T-42"}

	got := lineFor(japanese, eventTaskAssigned, actorAI, about)
	if want := "さくら が AMB-T-42 を さくら に割り当てました"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A name is reached for, never made up. A store holding no name for its user, and a facet that is
// neither of the two, are said the way they arrived.
func TestAFacetWithNoNameIsSaidAsItArrived(t *testing.T) {
	about := subject{name: "AMB-T-42"}
	nameless := preferences{language: "en", aiDisplayName: "AI"}

	if got, want := lineFor(nameless, eventTaskAssigned, actorHuman, about), "AI assigned AMB-T-42 to human"; got != want {
		t.Errorf("a user amenbo has no name for: got %q, want %q", got, want)
	}
	if got, want := lineFor(english, eventTaskAssigned, "committee", about), "AI assigned AMB-T-42 to committee"; got != want {
		t.Errorf("a facet this build does not know: got %q, want %q", got, want)
	}
}

// A status this build has no word for is still worth saying: the value off the wire says more than
// nothing, so a state amenbo adds later reports rather than disappearing.
func TestAStatusWithNoWordIsSaidAsItArrived(t *testing.T) {
	about := subject{name: "AMB-T-42"}

	got := lineFor(japanese, eventStatusChanged, "parked", about)
	if want := "さくら が AMB-T-42 をparkedに変更しました"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A twelfth event cannot be reached through hook, which filters on the catalog first. Naming it is
// still better than an empty message for a caller that hands one over.
func TestAnEventWithNoWordingIsStillNamed(t *testing.T) {
	about := subject{name: "AMB-T-42"}

	if got, want := lineFor(english, "task.pondered", "", about), "AI acted on AMB-T-42 (task.pondered)"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := lineFor(japanese, "task.pondered", "", about), "さくら が AMB-T-42 に対して操作しました（task.pondered）"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Every language amenbo offers has a row here, spelled the way amenbo spells the code — a `pt-br`
// where the store says `pt-BR` is a row nothing ever reads, and it would go out in English with
// nobody able to say why.
func TestEveryLanguageAmenboOffersHasARow(t *testing.T) {
	offered := []string{
		"de", "en", "es", "fr", "hi", "id", "it", "ja", "ko", "nl",
		"pl", "pt-BR", "ru", "th", "tr", "uk", "vi", "zh-Hans", "zh-Hant",
	}
	for _, code := range offered {
		if _, has := wordings[code]; !has {
			t.Errorf("no row for %q", code)
		}
	}
	if len(wordings) != len(offered) {
		t.Errorf("got %d rows for %d languages — one of them is spelled wrong", len(wordings), len(offered))
	}
}

// A third language, end to end, to show the row is wired and not merely present: the sentence, the
// name the user gave their AI, and amenbo's own word for the status, with the title untouched.
func TestAThirdLanguageIsSaidFromItsOwnRow(t *testing.T) {
	about := subject{name: "AMB-T-42", title: "Ship the thing"}
	german := preferences{language: "de", aiDisplayName: "Bob"}

	got := lineFor(german, eventStatusChanged, "in_progress", about)
	if want := "Bob hat AMB-T-42 auf In Arbeit gesetzt — Ship the thing"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Every language carries every sentence and every status. English is what a gap falls back to, so a
// gap would not show as a blank message — it would show as one language quietly said in another,
// which is exactly the kind of thing a test has to catch rather than a reader.
func TestEveryLanguageSaysEverything(t *testing.T) {
	statuses := []string{"todo", "in_progress", "done", "blocked", "rejected"}
	for language, words := range wordings {
		for _, event := range catalog {
			form, known := words.says[event]
			if !known {
				t.Errorf("%s: nothing to say about %s", language, event)
				continue
			}
			if form.bare == "" {
				t.Errorf("%s: %s has no sentence for what did not arrive", language, event)
			}
			for _, one := range []string{form.full, form.bare} {
				if one == "" {
					continue
				}
				if !strings.Contains(one, slotWhat) {
					t.Errorf("%s: %q should name what it is about", language, one)
				}
				// An event nobody drove has no subject to name, and naming one
				// would put a party into a sentence about a day arriving.
				if named := strings.Contains(one, slotWho); named == unattended[event] {
					t.Errorf("%s: %q should name who acted only if anybody did", language, one)
				}
			}
			// A comment's fuller form spends the record itself — the task it hangs on — so
			// the state is only owed by the events that carry one.
			if form.full != "" && carriesAState[event] && !strings.Contains(form.full, slotState) {
				t.Errorf("%s: %q is the fuller form and should spend the state", language, form.full)
			}
		}
		if !strings.Contains(words.unknown, slotEvent) {
			t.Errorf("%s: the sentence for an unknown event should name it: %q", language, words.unknown)
		}
		// The test message names nobody and nothing — a person pressed a button — so it is the
		// one line here with no slot in it, and holding it is holding that it exists.
		if words.test == "" {
			t.Errorf("%s: nothing to send as a test message", language)
		}
		for _, status := range statuses {
			if words.statuses[status] == "" {
				t.Errorf("%s: no word for the status %q", language, status)
			}
		}
	}
}

// carriesAState is the events amenbo hands a state to spend — the three whose fuller form has a
// `{state}` in it. The comment pair have a fuller form too, but what theirs spends is the record.
var carriesAState = map[string]bool{
	eventStatusChanged: true, eventTaskAssigned: true, eventTaskMoved: true,
}

// The five events that name a second thing are the five with a fuller form, and no other. A
// language filling `full` where nothing arrives would be writing a sentence with a hole in it.
func TestOnlyTheEventsThatCarryASecondThingHaveAFullerForm(t *testing.T) {
	carries := map[string]bool{
		eventCommentAdded: true, eventCommentRemoved: true,
	}
	for event := range carriesAState {
		carries[event] = true
	}
	for language, words := range wordings {
		for event, form := range words.says {
			if (form.full != "") != carries[event] {
				t.Errorf("%s: %s should have a fuller form only if it names a second thing (has %q)",
					language, event, form.full)
			}
		}
	}
}

// The comment events are the pair whose second thing is the record itself, so their fuller form is
// picked by the parent arriving rather than by a state.
func TestTheFormIsPickedByWhatTheEventCarries(t *testing.T) {
	parent := int64(7)
	for _, c := range []struct {
		in   input
		want bool
	}{
		{input{Event: eventStatusChanged, New: "todo"}, true},
		{input{Event: eventStatusChanged}, false},
		{input{Event: eventCommentAdded, Parent: &parent}, true},
		{input{Event: eventCommentAdded}, false},
		{input{Event: eventCommentAdded, New: "todo"}, false},
	} {
		if got := elaborated(c.in); got != c.want {
			t.Errorf("%s (new=%q, parent=%v): got %v, want %v", c.in.Event, c.in.New, c.in.Parent, got, c.want)
		}
	}
}
