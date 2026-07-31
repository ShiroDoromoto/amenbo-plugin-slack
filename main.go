// Command slack is amenbo's official Slack plugin: it reports to a Slack channel what the
// user's AI did in a project while nobody was watching it.
//
// It has one face, the observation hook. amenbo fires it with NO arguments and the event's
// JSON on stdin, nobody is waiting for the answer, and what it does with the event is send
// one message. There is no command face: nothing here is worth invoking on purpose, and a
// plugin that only observes says so by refusing an argument rather than by inventing a verb.
//
// Two things shape what it sends.
//
//   - **Only the AI's writes.** The payload names who drove the write, and a write the user
//     drove is one they were present for — a channel that repeats it back is noise. What is
//     worth a notification is the work that happened while they were away from the desk.
//   - **The channel is the setting.** `webhook_url` is a secret setting, and a setting
//     belongs to a project, so the value itself is which channel a project reports to.
//     There is nothing else to configure and no channel name anywhere in this code.
//
// A payload carries an id, never the record, so the title in a message is read back by
// running `amenbo task show <id> --json`. amenbo names the store and the window it may be
// read through in the environment; this plugin passes neither on and adds nothing of its own.
// The window is also who the message is from: the project it names is what a message leads
// with, and what tells one project's held lines from another's.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// contractVersion is the payload contract this plugin reads. amenbo leads every document it
// writes with `v` and raises it only on a breaking change — new fields are added silently —
// so a document announcing a different version is one this plugin must not guess at.
const contractVersion = 1

// actorAI is the one actor whose writes are reported. The other is the user themselves.
const actorAI = "ai"

// pluginName is what amenbo knows this plugin as: its manifest's name, its installed directory, and
// the word a user types after `plugin`. One spelling, so what is written under it is found again.
const pluginName = "slack"

// storeEnv names the store amenbo is working — the base directory it handed over on launch, which is
// also where this plugin's own installed directory sits.
const storeEnv = "AMENBO_HOME"

// webhookEnv carries the `webhook_url` setting. It is declared secret, so amenbo puts it in
// the environment rather than in the `config` object on stdin, under the name its key
// mechanically becomes.
const webhookEnv = "AMENBO_CONFIG_WEBHOOK_URL"

// out and errOut are the plugin's two channels, indirected so the tests can read what was
// written to each. A hook's stdout is not a return value, but the split still holds: nothing
// a person reads belongs anywhere but stderr, which is where amenbo's execution log looks
// when a run has to explain itself.
var (
	out    io.Writer = os.Stdout
	errOut io.Writer = os.Stderr
)

// logf writes one diagnostic line to stderr.
func logf(format string, a ...any) {
	fmt.Fprintf(errOut, format+"\n", a...)
}

// input is the JSON document amenbo writes to the plugin's stdin. Unknown keys are ignored —
// the contract grows by addition, so a plugin that refused them would break on the next
// amenbo.
type input struct {
	// V is the contract version the document is written to.
	V int `json:"v"`
	// Event is the event's namespace name, e.g. "task.done". Empty when nothing fired.
	Event string `json:"event"`
	// ID is the affected record's conversational number — the id a person knows it by.
	ID int64 `json:"id"`
	// Actor is who drove the write: "human" or "ai".
	Actor string `json:"actor"`
	// At is when the event fired, as "2026-07-22T09:00:00Z". Redelivery of one event carries the
	// same moment, which is what tells a replay from the user acting twice.
	At string `json:"at"`
	// New is the record's state after the change, for the events whose name does not
	// already say it.
	New string `json:"new"`
	// Record is the vanished record itself, on the deletion events alone: there is nothing
	// left to read back, so the row travels on the wire in its place. Its keys are the
	// record's own columns.
	Record map[string]any `json:"record"`
	// Parent is what a child record hangs on, by number — the task of a comment, added or
	// taken back. Nil on every event that has no parent, and on an older amenbo that carried
	// none for one that does.
	Parent *int64 `json:"parent"`
	// Config holds the plugin's own non-secret settings, as the user filled them in. Secrets
	// never appear here: amenbo puts those in the environment instead.
	Config map[string]any `json:"config"`
}

// recordField reads one field of the vanished record as text. A value that is not a string is
// not one this plugin can put in a sentence, and reads as absent rather than being coerced.
func (in input) recordField(key string) string {
	text, _ := in.Record[key].(string)
	return strings.TrimSpace(text)
}

// readInput reads the document amenbo feeds on stdin.
//
// amenbo always writes one and closes the pipe, so the read finishes promptly. A hand run
// from a terminal is fed nothing at all, and waiting for a person to type JSON would hang
// the plugin on `slack help` — so an interactive stdin is skipped rather than read. A
// document that will not parse is reported and dropped: nobody is waiting on the answer, and
// there is no event left to report.
func readInput(f *os.File) input {
	if info, err := f.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
		return input{}
	}
	raw, err := io.ReadAll(f)
	if err != nil || len(bytes.TrimSpace(raw)) == 0 {
		return input{}
	}
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		logf("slack: ignoring an input document that will not parse: %v", err)
		return input{}
	}
	return in
}

func main() {
	in := readInput(os.Stdin)
	args := os.Args[1:]

	// No arguments is the observation face — amenbo fired us for an event.
	if len(args) == 0 {
		do(hook(in))
		return
	}

	switch args[0] {
	case "help", "-h", "--help":
		usage()
	default:
		logf("slack: %q is not a command — this plugin only reports events", strings.Join(args, " "))
		usage()
		os.Exit(2)
	}
}

// do ends the run on the verdict the exit code carries. A hook's failure reaches nobody who
// was listening, so the exit code is what puts it in amenbo's execution log, beside the
// stderr that says why.
func do(err error) {
	if err != nil {
		logf("slack: %v", err)
		os.Exit(1)
	}
}

func usage() {
	logf(`slack — amenbo's official plugin: report your AI's writes to a Slack channel

This plugin is not called. amenbo starts it when an event fires, and it reports the event as one
line, under a heading naming the project it came from. Lines wait while amenbo says more events are
queued, so a burst — a project deleted, a pile cleared — arrives as one message rather than tens.

Only the writes an AI drove are reported: the ones you drove yourself, you were there for.
Which of them reach the channel is yours to choose — by default a task created, its status
moved, and either terminal (done or decided against).

A message is written in the language amenbo is set to, and its subject is the name you gave your
AI. Neither is a setting here — both are read back from amenbo, and a language this build has no
wording for is reported in English.

Settings:
  webhook_url   the Slack incoming webhook to post to (secret, required)
  events        what to report, from the eleven amenbo fires (defaults to the four above;
                choosing none is honoured, and reports nothing)

The setting belongs to a project, so the value is which channel that project reports to.
Fill it in with 'amenbo plugin config set slack webhook_url <url>', then switch the plugin
on for the project with 'amenbo plugin enable slack'.

Why nothing arrived is in 'amenbo plugin log slack' — one line per run, and the diagnostics
of any run that did not end cleanly.`)
}
