package main

import (
	"fmt"
	"os"
	"strings"
)

// The v1 event catalog, in the order the manifest declares it. Every one of them is both an
// answer the user can pick and an event the manifest subscribes to: a subscription is settled at
// install time, so subscribing to all of them and filtering here is what makes *what to report* a
// setting the user can change their mind about, per project, whenever they like.
const (
	eventTaskCreated      = "task.created"
	eventStatusChanged    = "task.status_changed"
	eventTaskDone         = "task.done"
	eventTaskRejected     = "task.rejected"
	eventTaskAssigned     = "task.assigned"
	eventTaskMoved        = "task.moved"
	eventTaskDeleted      = "task.deleted"
	eventDecisionAccepted = "decision.accepted"
	eventDecisionRejected = "decision.rejected"
	eventCommentAdded     = "comment.added"
	eventCommentRemoved   = "comment.removed"
)

// catalog is that list, and a test holds it against the manifest — the events subscribed to and
// the answers offered are the same eleven, or an option is a choice that reports nothing.
var catalog = []string{
	eventTaskCreated,
	eventStatusChanged,
	eventTaskDone,
	eventTaskRejected,
	eventTaskAssigned,
	eventTaskMoved,
	eventTaskDeleted,
	eventDecisionAccepted,
	eventDecisionRejected,
	eventCommentAdded,
	eventCommentRemoved,
}

// defaultEvents is what a channel gets while nobody has said otherwise: a task appeared, its
// status moved, and either terminal — the shape of "what happened while I was away". The manifest
// declares the same four as the setting's default, and a test holds those together too.
var defaultEvents = []string{eventTaskCreated, eventStatusChanged, eventTaskDone, eventTaskRejected}

// eventsSetting is the key the choice arrives under. It is not a secret, so it comes in the
// `config` object on stdin rather than in the environment.
const eventsSetting = "events"

// hook is the observation face. Every launch does two things, and they are separate: it may turn this
// event into a line, and it may send what is owed.
//
// **Taking the event in.** Four filters stand in front of that, and each one is a silence rather than
// a failure — a document from a contract this plugin cannot read, a write the user drove themselves,
// an event the user did not ask to hear about, and an event already taken in are all events with
// nothing to add. What is added is written down before anything else happens, so a launch that does
// not come back has not swallowed it.
//
// **Sending what is owed.** This is the runner's question, not the event's: while anything is still
// queued for this plugin, the lines wait; when nothing is, they go out as one message. Which is why
// the flush is not behind those filters — a burst that ends in events nobody asked to hear about
// would otherwise leave the lines in front of them waiting for the next reportable write, which may
// be hours away or never (found on a real store: four creations followed by four deletions nobody
// subscribed to, and three lines stranded).
//
// What can go wrong is the send, and the reads behind it. The two are not the same failure: a webhook
// that will not take the message means nothing was reported — and what was owed stays owed, so the
// next flush carries it — while a record that could not be read back costs one line its title, and a
// project that could not be read costs the message its heading. So the message goes out either way,
// carrying what was readable, and the run still ends non-zero so the fault lands in the execution
// log instead of quietly shortening every message from here on.
func hook(in input) error {
	// State is kept per project now; a run under the split leaves nothing of the older shape behind.
	dropLegacy()
	batch := held()
	var readErr, holdErr, takeErr error

	if reportable(in) {
		taken := recall()
		key := takenKey(in)
		// An event delivered twice adds no second line, and the flush below still runs: this
		// launch may be the one with nothing behind it (see taken.go).
		if !taken.holds(key) {
			var about subject
			about, readErr = describe(in)
			batch.add(sentence(in.Event, in.New, about))
			// Nothing from here on may stop the send. A store that cannot be written is a
			// fault worth a failed run, but holding a line back on account of it would mean
			// carrying it in a process that is about to end — so the batching is what gives
			// way, not the notification.
			if holdErr = batch.save(); holdErr != nil {
				batch.loosen()
			}
			// Recorded as taken in, once it is as safely held as it is going to be: from
			// here on this event has been dealt with, whether its line goes out in this
			// run's message or a later one.
			takeErr = taken.add(key)
		}
	}

	if len(batch.messages) == 0 {
		return firstFault(holdErr, takeErr)
	}
	if batch.durable() && remaining() > 0 {
		return firstFault(takeErr, readFailure(readErr))
	}

	webhook := os.Getenv(webhookEnv)
	if webhook == "" {
		// `required` keeps the plugin from being enabled while this is empty, so arriving
		// here means the value was taken away from underneath a gate that is already open.
		return fmt.Errorf("no webhook to post to — set it with 'amenbo plugin config set slack webhook_url <url>'")
	}
	head, headErr := heading()
	if err := post(webhook, head+batch.text()); err != nil {
		return err
	}
	clearErr := batch.clear()
	return firstFault(holdErr, takeErr, clearErr, readFailure(readErr), readFailure(headErr))
}

// heading is the line a message leads with: the project it came from, in bold.
//
//	*amenbo-plugin-slack*
//	AI created AMB-T-42 — Ship the thing
//
// A webhook belongs to a project, so a channel used to be the answer to which project a line is
// about. It stops being one the moment two projects are pointed at the same channel, and it was
// never one in a notification preview. The name is said once at the top rather than on every line,
// because every line in a batch is from the same project — the state they were held in is that
// project's.
//
// It is read when a message is sent, not when a line is taken in: a launch that only holds a line
// has nothing to head, and reading then would spend a read per event to say one thing per message.
//
// Two answers are not a heading and are not a fault either: a launch with no project named is a run
// by hand, and a project that answers with no name has nothing to put there. A read that failed is
// a fault — but it costs the message its heading and nothing else, the same way a title that could
// not be read costs one line its title.
func heading() (string, error) {
	reach := strings.TrimSpace(os.Getenv(reachEnv))
	if reach == "" {
		return "", nil
	}
	name, err := projectShow(reach)
	if err != nil || name == "" {
		return "", err
	}
	return "*" + name + "*\n", nil
}

// reportable says whether this event becomes a line: one this plugin can read, driven by the AI, and
// asked for.
func reportable(in input) bool {
	return in.V == contractVersion && in.Actor == actorAI && selected(in)[in.Event]
}

// readFailure is what a run ends on when the record behind a line could not be read: the line was
// still delivered, so this is a fault to log rather than work to redo.
func readFailure(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("the message went out without what it could not read: %w", err)
}

// firstFault is the one a run ends on. Every fault here is about bookkeeping around a message that
// did go out, so they are alike in what they cost: the exit code puts the run in the execution log,
// and the first reason is the one worth reading.
func firstFault(faults ...error) error {
	for _, fault := range faults {
		if fault != nil {
			return fault
		}
	}
	return nil
}

// selected is the set of events this run may report.
//
// The setting has three answers and they arrive already resolved — a multi field's bookkeeping
// never reaches a plugin: the values chosen, joined by commas; the declared default, while the
// user has not answered; and **empty, when they chose none deliberately**. Empty is honoured as
// it is meant rather than read as "unset" and quietly replaced by the default, which is the whole
// reason a deliberate none is kept apart from an unanswered field.
//
// A `config` carrying no `events` key at all is the one case that is not an answer: an amenbo
// from before the setting, or a manifest that does not declare it. There the built-in default
// stands, so such a build reports what it has always reported.
func selected(in input) map[string]bool {
	answer, declared := in.Config[eventsSetting].(string)
	if !declared {
		return setOf(defaultEvents)
	}
	chosen := map[string]bool{}
	for _, name := range strings.Split(answer, ",") {
		if name = strings.TrimSpace(name); name != "" {
			chosen[name] = true
		}
	}
	return chosen
}

// setOf is a list of event names as a set.
func setOf(names []string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, name := range names {
		set[name] = true
	}
	return set
}

// subject is what a message is about, split where a sentence needs it split: the name a reader
// can look the record up by, and the title, which always trails — so a long one never lands in
// the middle and pushes the state that changed out of sight.
type subject struct {
	name  string
	title string
}

// describe names what a line is about, reading back whatever the payload only named. Which read
// that is depends on the event, and one of them cannot be read at all:
//
//   - a live task or decision is read back by its id — the ordinary case
//   - a **deleted** task is gone, so its title comes off the vanished record the payload carries
//     in its place
//   - a comment, added or taken back, is named by the task it hangs on — the payload carries
//     that as the parent — rather than by its own number, which is a handle inside the store's
//     timeline and points at nothing a reader in a channel can follow
func describe(in input) (subject, error) {
	switch in.Event {
	case eventTaskDeleted:
		return subject{name: fmt.Sprintf("task #%d", in.ID), title: in.recordField("title")}, nil
	case eventDecisionAccepted, eventDecisionRejected:
		ref, title, err := decisionShow(in.ID)
		return named(fmt.Sprintf("decision #%d", in.ID), ref, title), err
	case eventCommentAdded:
		return commentOn(in, fmt.Sprintf("comment #%d", in.ID))
	case eventCommentRemoved:
		return commentOn(in, fmt.Sprintf("a comment (#%d)", in.ID))
	default:
		ref, title, err := taskShow(in.ID)
		return named(fmt.Sprintf("task #%d", in.ID), ref, title), err
	}
}

// commentOn names a comment by the task it hangs on, which is the only end of it a reader can
// pick up: the comment's own number belongs to a timeline they are not looking at.
//
// The parent is a field that was added to the payload rather than one whose meaning changed, so
// an amenbo old enough to send none is not a version to refuse over — it is a payload with less
// in it, and the message falls back to naming the comment by its number, which is all there is.
func commentOn(in input, fallback string) (subject, error) {
	if in.Parent == nil {
		return subject{name: fallback}, nil
	}
	ref, title, err := taskShow(*in.Parent)
	on := named(fmt.Sprintf("task #%d", *in.Parent), ref, title)
	return subject{name: "a comment on " + on.name, title: on.title}, err
}

// named is how a record read back is pointed at: its ref and title where the read answered, and
// the caller's fallback where it did not. The number is always known — it is what the payload
// carries — so there is no event this plugin cannot name.
func named(fallback, ref, title string) subject {
	if ref == "" {
		return subject{name: fallback}
	}
	return subject{name: ref, title: title}
}

// sentence is the message: what the AI did, to which record.
func sentence(event, newState string, about subject) string {
	var said string
	switch event {
	case eventTaskCreated:
		said = "AI created " + about.name
	case eventTaskDone:
		said = "AI finished " + about.name
	case eventTaskRejected:
		said = "AI decided against " + about.name
	case eventStatusChanged:
		said = "AI moved " + about.name
		if newState != "" {
			said += " to " + newState
		}
	case eventTaskAssigned:
		said = "AI assigned " + about.name
		if newState != "" {
			said += " to " + newState
		}
	case eventTaskMoved:
		said = "AI moved " + about.name
		if newState != "" {
			said += " into " + newState
		} else {
			said += " to another project"
		}
	case eventTaskDeleted:
		said = "AI deleted " + about.name
	case eventDecisionAccepted:
		said = "AI accepted " + about.name
	case eventDecisionRejected:
		said = "AI rejected " + about.name
	case eventCommentAdded:
		said = "AI added " + about.name
	case eventCommentRemoved:
		said = "AI took back " + about.name
	default:
		// Out of reach through hook, which filters on the catalog first. Naming the event is
		// still better than an empty message for a caller that hands over a twelfth one.
		said = "AI acted on " + about.name + " (" + event + ")"
	}
	if about.title == "" {
		return said
	}
	return said + " — " + about.title
}
