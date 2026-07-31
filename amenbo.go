package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// amenboCmd is the binary a plugin reads records back with. There is no second protocol and
// no library to link — a plugin is any executable, so the one route that works in every
// language is amenbo itself, on the PATH the user already has it on.
const amenboCmd = "amenbo"

// runAmenbo runs one amenbo command and returns its stdout. Indirected so a test can stand in
// for the binary without one on the PATH.
//
// The environment is inherited untouched, and that is the whole of the read-back path: amenbo
// hands a plugin the store to open and the window to read it through when it launches it,
// because neither can be worked out from where a plugin stands — its working directory is
// whatever its launcher happened to be in, and there is no binding of its own beneath it.
var runAmenbo = func(args ...string) ([]byte, error) {
	cmd := exec.Command(amenboCmd, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		said := strings.TrimSpace(stderr.String())
		if said == "" {
			said = err.Error()
		}
		return nil, fmt.Errorf("%s %s: %s", amenboCmd, strings.Join(args, " "), said)
	}
	return stdout.Bytes(), nil
}

// actorFlag declares the facet the read is made under. Every operation that uses the facet
// requires it and never defaults it, and a read is one of them — it is what an AI's reach is
// otherwise drawn from. Here it is not: amenbo handed the window over in the environment, so
// what the flag settles is only that the facet was declared, and `ai` is the narrower of the
// two to declare.
var actorFlag = []string{"--actor", "ai"}

// taskShow reads back the two things a message needs about a task: the ref a reader can look it
// up by, and its title. A payload carries neither — it names the record and stops there.
func taskShow(id int64) (ref, title string, err error) {
	return show("task", id)
}

// decisionShow is the same read on the other record a v1 event can name.
func decisionShow(id int64) (ref, title string, err error) {
	return show("decision", id)
}

// projectShow reads back the name of the project a run reaches. It is named by the ref amenbo hands
// over rather than by a number, and it is the one read here that is not about a record the payload
// carried: what it answers is who the message is from.
func projectShow(reach string) (name string, err error) {
	args := append([]string{"project", "show", reach, "--json"}, actorFlag...)
	raw, err := runAmenbo(args...)
	if err != nil {
		return "", err
	}
	var record struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &record); err != nil {
		return "", fmt.Errorf("reading back project %s: %w", reach, err)
	}
	return record.Name, nil
}

// show reads one record back — `amenbo <kind> show <id> --json` — and takes the two fields a
// message is built from. A task and a decision answer with the same two names, so one reader
// covers both.
//
// A failure here is returned rather than raised: what it costs is the title, not the message
// (see hook). The refusals worth expecting are a window that does not cover this record and a
// store the caller's amenbo cannot open, and both say so on stderr, which is carried into the
// error so the execution log holds the reason and not just the fact.
func show(kind string, id int64) (ref, title string, err error) {
	args := append([]string{kind, "show", strconv.FormatInt(id, 10), "--json"}, actorFlag...)
	raw, err := runAmenbo(args...)
	if err != nil {
		return "", "", err
	}
	var record struct {
		Ref   string `json:"ref"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(raw, &record); err != nil {
		return "", "", fmt.Errorf("reading back %s %d: %w", kind, id, err)
	}
	return record.Ref, record.Title, nil
}

// preferences is how a message should read: the language to say it in, and the names the two who
// act in it go by. None of it is on the wire — a payload carries what happened, not how to word
// it — and none of it is a setting of this plugin's own: the user has already told amenbo all
// three, and asking again here would be the same question answered in two places, free to
// disagree.
type preferences struct {
	// language is amenbo's language code, e.g. "ja", passed on as it was answered. Which codes
	// there is a wording for is the wording's business; the read does not judge the answer,
	// so a language amenbo adds later arrives here rather than being refused on the way in.
	language string
	// aiDisplayName is the name the AI goes by — the subject of every sentence this plugin writes.
	aiDisplayName string
	// humanDisplayName is the name the user goes by, which a line needs when the AI hands work
	// over rather than does it. Unlike the AI's, it has no fallback here: amenbo already answers
	// with a name of its own — in the language it is set to — while the user has not chosen one,
	// so there is nothing left for this side to invent. Empty is what a store that could not be
	// read at all leaves, and then the facet is said as it arrived.
	humanDisplayName string
}

// defaultPreferences is how a message reads while the store has not said otherwise: English, and
// the AI named the way amenbo names it out of the box.
var defaultPreferences = preferences{language: fallbackLanguage, aiDisplayName: "AI"}

// readPreferences reads them back — `amenbo config --json` — through the same route and the same
// declared facet as a title. All of it comes off one answer, so a name costs no read of its own.
//
// Read it once, where the line is worded, and carry the answer from there: it is one answer per
// message, not one per line, and the read costs a launch of amenbo either way.
//
// A failure is a fault the run can log, and nothing more: the caller is handed the fallback
// alongside it and words the message with that, the same way a title that could not be read costs
// one line its title rather than the message. An answer that came back empty is the same case
// arriving by another road — a setting cleared to nothing is not a language, and not a name to put
// at the head of a sentence — so it falls back too, and quietly, being an answer rather than a
// refusal.
func readPreferences() (preferences, error) {
	args := append([]string{"config", "--json"}, actorFlag...)
	raw, err := runAmenbo(args...)
	if err != nil {
		return defaultPreferences, err
	}
	var record struct {
		Settings struct {
			Language         string `json:"language"`
			AIDisplayName    string `json:"ai_display_name"`
			HumanDisplayName string `json:"human_display_name"`
		} `json:"settings"`
	}
	if err := json.Unmarshal(raw, &record); err != nil {
		return defaultPreferences, fmt.Errorf("reading back the settings: %w", err)
	}
	return preferences{
		language:         orElse(record.Settings.Language, defaultPreferences.language),
		aiDisplayName:    orElse(record.Settings.AIDisplayName, defaultPreferences.aiDisplayName),
		humanDisplayName: strings.TrimSpace(record.Settings.HumanDisplayName),
	}, nil
}

// orElse takes the store's answer where there is one, and the fallback where there is not.
func orElse(answer, otherwise string) string {
	if answer = strings.TrimSpace(answer); answer != "" {
		return answer
	}
	return otherwise
}
