package main

import (
	"fmt"
	"os"
)

// The events this plugin reports. They are the writes a person away from their desk wants to
// hear about: work that appeared, work that ended either way, and work that started moving.
// The same four are what the manifest subscribes to, and a test holds the two lists together
// — an event named in one place only is either a message nobody asked for or a launch that
// reports nothing.
const (
	eventTaskCreated   = "task.created"
	eventStatusChanged = "task.status_changed"
	eventTaskDone      = "task.done"
	eventTaskRejected  = "task.rejected"
)

// reported is that list, in the order the manifest declares it.
var reported = []string{eventTaskCreated, eventStatusChanged, eventTaskDone, eventTaskRejected}

// hook is the observation face: amenbo fired an event, and this is the message it becomes.
//
// Three filters stand in front of the send, and each one is a silence rather than a failure —
// a document from a contract this plugin cannot read, a write the user drove themselves, and
// an event outside the four above are all runs with nothing to say, not runs that went wrong.
//
// What can go wrong is the send, and the read behind it. The two are not the same failure: a
// webhook that will not take the message means nothing was reported, while a title that could
// not be read back costs the message its title and nothing else. So the message goes out
// either way, carrying what was readable, and the run still ends non-zero so the fault lands
// in the execution log instead of quietly shortening every message from here on.
func hook(in input) error {
	if in.V != contractVersion {
		return nil
	}
	if in.Actor != actorAI {
		return nil
	}
	if !reports(in.Event) {
		return nil
	}

	webhook := os.Getenv(webhookEnv)
	if webhook == "" {
		// `required` keeps the plugin from being enabled while this is empty, so arriving
		// here means the value was taken away from underneath a gate that is already open.
		return fmt.Errorf("no webhook to post to — set it with 'amenbo plugin config set slack webhook_url <url>'")
	}

	ref, title, readErr := taskShow(in.ID)
	if err := post(webhook, sentence(in.Event, in.New, taskName(in.ID, ref), title)); err != nil {
		return err
	}
	if readErr != nil {
		return fmt.Errorf("the message went out without the task's title: %w", readErr)
	}
	return nil
}

// reports says whether an event is one of the four.
func reports(event string) bool {
	for _, name := range reported {
		if name == event {
			return true
		}
	}
	return false
}

// taskName is how a message points at a task: the ref read back from the store, or the bare
// number when nothing could be read. The number is always known — it is what the payload
// carries — so there is no event this plugin cannot name.
func taskName(id int64, ref string) string {
	if ref != "" {
		return ref
	}
	return fmt.Sprintf("task #%d", id)
}

// sentence is the message: what the AI did, to which task. The title comes last on every
// event, so a long one never sits in the middle of the sentence and pushes the state that
// changed out of sight. A title nobody could read back is simply absent.
func sentence(event, newState, task, title string) string {
	var said string
	switch event {
	case eventTaskCreated:
		said = "AI created " + task
	case eventTaskDone:
		said = "AI finished " + task
	case eventTaskRejected:
		said = "AI decided against " + task
	case eventStatusChanged:
		said = "AI moved " + task
		if newState != "" {
			said += " to " + newState
		}
	default:
		// Out of reach through hook, which filters on the four first. Naming the event is
		// still better than an empty message for a caller that hands over a fifth one.
		said = "AI acted on " + task + " (" + event + ")"
	}
	if title == "" {
		return said
	}
	return said + " — " + title
}
