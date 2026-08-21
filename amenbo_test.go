package main

import (
	"strings"
	"testing"
)

// answersWith stands in for an Amenbo that hands back one document to whatever is asked of it,
// and collects what was asked.
func answersWith(t *testing.T, document string) *[]string {
	t.Helper()
	var asked []string
	previous := runAmenbo
	runAmenbo = func(args ...string) ([]byte, error) {
		asked = append(asked, strings.Join(args, " "))
		return []byte(document), nil
	}
	t.Cleanup(func() { runAmenbo = previous })
	return &asked
}

func TestPreferencesAreReadBackFromTheStore(t *testing.T) {
	asked := answersWith(t, `{"settings":{"language":"ja","ai_display_name":"さくら","human_display_name":"山田"}}`)

	got, err := readPreferences()
	if err != nil {
		t.Fatalf("an answer this plain should not fault: %v", err)
	}
	if got.language != "ja" {
		t.Errorf("the language should be the store's answer, got %q", got.language)
	}
	if got.aiDisplayName != "さくら" {
		t.Errorf("the AI should go by the name the store gives it, got %q", got.aiDisplayName)
	}
	// Both names come off the one answer, so naming the user costs no read of its own.
	if got.humanDisplayName != "山田" {
		t.Errorf("the user should go by the name the store gives them, got %q", got.humanDisplayName)
	}
	// One read, declaring the facet: every operation that uses one requires it.
	want := []string{"config --json --actor ai"}
	if len(*asked) != len(want) || (*asked)[0] != want[0] {
		t.Errorf("read %q, want %q", *asked, want)
	}
}

// A code this build has no wording for is still the store's answer. Falling back is what the
// wording does when it cannot find one, so the read passes the code on untouched — which is how
// a language Amenbo adds later reaches a build that predates it.
func TestAnUnknownLanguageIsPassedOnAsAnswered(t *testing.T) {
	answersWith(t, `{"settings":{"language":"eo","ai_display_name":"AI"}}`)

	got, err := readPreferences()
	if err != nil {
		t.Fatalf("an unknown code is an answer, not a fault: %v", err)
	}
	if got.language != "eo" {
		t.Errorf("the code should arrive as it was answered, got %q", got.language)
	}
}

// An answer that carries neither, or carries them cleared to nothing, is an answer — so the
// fallback stands in and the run is not faulted for it.
func TestPreferencesFallBackOnAnAnswerWithoutThem(t *testing.T) {
	for name, document := range map[string]string{
		"no settings at all":    `{"app_version":"3.1.0"}`,
		"settings without them": `{"settings":{"onboarded":true}}`,
		"answered as nothing":   `{"settings":{"language":null,"ai_display_name":null,"human_display_name":null}}`,
		"cleared to blanks":     `{"settings":{"language":"  ","ai_display_name":"","human_display_name":"  "}}`,
	} {
		t.Run(name, func(t *testing.T) {
			answersWith(t, document)

			got, err := readPreferences()
			if err != nil {
				t.Fatalf("an answer without them is not a fault: %v", err)
			}
			if got != defaultPreferences {
				t.Errorf("got %+v, want the fallback %+v", got, defaultPreferences)
			}
		})
	}
}

// A store that would not answer costs the message its language and nothing else: the fallback
// comes back with the fault, so the caller has something to word the line with either way.
func TestPreferencesFallBackWhenTheStoreWillNotAnswer(t *testing.T) {
	refusesToRead(t, "out_of_reach")

	got, err := readPreferences()
	if err == nil {
		t.Fatal("a read that failed should be a fault the run can log")
	}
	if !strings.Contains(err.Error(), "out_of_reach") {
		t.Errorf("the reason should reach the log, got %v", err)
	}
	if got != defaultPreferences {
		t.Errorf("got %+v, want the fallback %+v", got, defaultPreferences)
	}
}

func TestPreferencesFallBackOnAnAnswerThatWillNotParse(t *testing.T) {
	answersWith(t, `{"settings":`)

	got, err := readPreferences()
	if err == nil {
		t.Fatal("an answer that will not parse should be a fault the run can log")
	}
	if got != defaultPreferences {
		t.Errorf("got %+v, want the fallback %+v", got, defaultPreferences)
	}
}

// The user's name is the one field with no fallback: Amenbo names its AI out of the box and its
// user only when they say so, so an unanswered one stays empty and the facet is said as it
// arrived, rather than a person being called something nobody chose.
func TestTheUsersNameIsLeftEmptyRatherThanInvented(t *testing.T) {
	answersWith(t, `{"settings":{"language":"en","ai_display_name":"AI"}}`)

	got, err := readPreferences()
	if err != nil {
		t.Fatalf("an answer without it is not a fault: %v", err)
	}
	if got.humanDisplayName != "" {
		t.Errorf("nothing should be made up here, got %q", got.humanDisplayName)
	}
}
