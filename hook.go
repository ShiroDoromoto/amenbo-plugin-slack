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

// hook is the observation face: amenbo fired an event, and this is the message it becomes.
//
// Four filters stand in front of the send, and each one is a silence rather than a failure — a
// document from a contract this plugin cannot read, a write the user drove themselves, an event they
// did not ask to hear about, and an event that was already sent are all runs with nothing to say,
// not runs that went wrong.
//
// What can go wrong is the send, and the read behind it. The two are not the same failure: a
// webhook that will not take the message means nothing was reported, while a record that could
// not be read back costs the message its title and nothing else. So the message goes out either
// way, carrying what was readable, and the run still ends non-zero so the fault lands in the
// execution log instead of quietly shortening every message from here on.
func hook(in input) error {
	if in.V != contractVersion {
		return nil
	}
	if in.Actor != actorAI {
		return nil
	}
	if !selected(in)[in.Event] {
		return nil
	}
	// Before anything is read or sent: an event delivered twice is one message, and the second
	// delivery is amenbo's bookkeeping rather than news (see sent.go).
	sent := recall()
	key := sentKey(in)
	if sent.holds(key) {
		return nil
	}

	webhook := os.Getenv(webhookEnv)
	if webhook == "" {
		// `required` keeps the plugin from being enabled while this is empty, so arriving
		// here means the value was taken away from underneath a gate that is already open.
		return fmt.Errorf("no webhook to post to — set it with 'amenbo plugin config set slack webhook_url <url>'")
	}

	about, readErr := about(in)
	if err := post(webhook, sentence(in.Event, in.New, about)); err != nil {
		return err
	}
	// Recorded only once the message is out: a send that failed was not a delivery, so a replay of
	// it should carry the message rather than skip it.
	if err := sent.add(key); err != nil {
		return fmt.Errorf("the message went out, but this run may repeat it: %w", err)
	}
	if readErr != nil {
		return fmt.Errorf("the message went out without what it could not read: %w", readErr)
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

// about names what the message is about, reading back whatever the payload only named. Which read
// that is depends on the event, and one of them cannot be read at all:
//
//   - a live task or decision is read back by its id — the ordinary case
//   - a **deleted** task is gone, so its title comes off the vanished record the payload carries
//     in its place
//   - a comment **added** is named by its own number: nothing on the wire says which task it
//     hangs on, and there is nothing to ask for one with
//   - a comment **taken back** does name its task — the payload carries it as the parent — so
//     that one is read back after all
func about(in input) (subject, error) {
	switch in.Event {
	case eventTaskDeleted:
		return subject{name: fmt.Sprintf("task #%d", in.ID), title: in.recordField("title")}, nil
	case eventDecisionAccepted, eventDecisionRejected:
		ref, title, err := decisionShow(in.ID)
		return named(fmt.Sprintf("decision #%d", in.ID), ref, title), err
	case eventCommentAdded:
		return subject{name: fmt.Sprintf("comment #%d", in.ID)}, nil
	case eventCommentRemoved:
		if in.Parent == nil {
			return subject{name: fmt.Sprintf("a comment (#%d)", in.ID)}, nil
		}
		ref, title, err := taskShow(*in.Parent)
		on := named(fmt.Sprintf("task #%d", *in.Parent), ref, title)
		return subject{name: "a comment on " + on.name, title: on.title}, err
	default:
		ref, title, err := taskShow(in.ID)
		return named(fmt.Sprintf("task #%d", in.ID), ref, title), err
	}
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
