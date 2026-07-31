package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// A plugin cannot see its own queue: it is launched once per event, and deleting a project can put
// fifty of them on at once. What the runner does hand over is how many are still behind this one, so
// a burst that is one act to the user — a project deleted, a pile cleared — can arrive as one
// message instead of fifty.
//
// So a message is held while there is anything behind it, and the run that sees nothing behind it
// sends everything held as one. Three things about that shape are worth stating outright:
//
//   - **The held messages are this plugin's to carry.** A launch ends after one event, and the row
//     it was for has left the queue — nothing is waiting on amenbo's side, so what is held has to be
//     on disk before anything else happens, or a run that never comes back takes it with it.
//   - **The number counts this launch's project and no other.** A launch reaches one project, holds
//     that project's lines and posts to that project's channel, so the count it flushes on is that
//     project's too. Two projects firing at once neither delay nor flush each other.
//   - **Zero is not a promise that nothing more is coming.** An event written a moment later is
//     delivered like any other, so a batch flushed on zero may be followed by a second one. That is
//     one message becoming two, never a message lost.

// pendingFile holds the messages taken in but not yet sent, one JSON string per line — JSON because a
// title is the user's text and could carry anything, newlines included, and a line that broke in two
// would arrive as two messages.
const pendingFile = "pending-messages.log"

// reachQueueRemainingEnv is how the runner says how much is behind this launch, within the project
// this launch fires for. It is the same scope as the reach itself, which is what the name says.
const reachQueueRemainingEnv = "AMENBO_PLUGIN_REACH_QUEUE_REMAINING"

// remaining is how many events are still queued after this one for the project this launch fires for.
//
// A runner counts once at the start of a pass and hands out that count decreasing, so the numbers
// reach zero however long the pass is. The count is this project's alone, which is what makes a zero
// worth flushing on: the lines held here belong to this project too, and no other project's events
// can keep them waiting. Nothing said, or something that will not parse as a count, is read as zero:
// an amenbo too old to carry the variable, and a run by hand, both mean "there is nothing behind
// this" — one message per event, which is a plainer channel and never a message lost.
func remaining() int {
	count, err := strconv.Atoi(strings.TrimSpace(os.Getenv(reachQueueRemainingEnv)))
	if err != nil || count < 0 {
		return 0
	}
	return count
}

// pending is what has been taken in and not yet sent: whatever earlier runs held, in the order they
// held it.
type pending struct {
	// path is where the messages are kept, empty when there is nowhere to keep them.
	path     string
	messages []string
}

// held reads what is owed. A file that cannot be read comes back empty rather than refusing the run —
// the cost is a message that stays unsent, and refusing would add one that never went out at all.
func held() pending {
	path := statePath(pendingFile)
	if path == "" {
		return pending{}
	}
	owed := pending{path: path}
	for _, line := range readLines(path) {
		var message string
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			// A line this plugin cannot read is one it cannot send; dropping it loses one
			// message, while stopping here would lose every message behind it too.
			logf("slack: skipping a held message that will not parse: %v", err)
			continue
		}
		owed.messages = append(owed.messages, message)
	}
	return owed
}

// durable says whether what is held will survive this run. Where there is nowhere to write, holding
// a message would mean carrying it in a process that is about to end — so nothing is held back at
// all, and every event is its own message.
func (p pending) durable() bool {
	return p.path != ""
}

// add puts one message at the end of what is owed.
func (p *pending) add(message string) {
	p.messages = append(p.messages, message)
}

// loosen gives up keeping what is owed: nothing will be held back, so every message goes out as it
// arrives. What it costs is the batching, which is the right thing to lose — a message held in a
// process about to end is a message nobody ever gets.
func (p *pending) loosen() {
	p.path = ""
}

// save writes what is owed down, so a run that does not come back has not swallowed it.
func (p pending) save() error {
	if !p.durable() {
		return nil
	}
	lines := make([]string, 0, len(p.messages))
	for _, message := range p.messages {
		line, err := json.Marshal(message)
		if err != nil {
			return fmt.Errorf("holding a message for the rest of the queue: %w", err)
		}
		lines = append(lines, string(line))
	}
	if err := writeWhole(p.path, lines); err != nil {
		return fmt.Errorf("holding a message for the rest of the queue: %w", err)
	}
	return nil
}

// text is everything owed as one message: one line each, in the order they happened.
func (p pending) text() string {
	return strings.Join(p.messages, "\n")
}

// clear forgets what is owed, once it has gone out.
func (p pending) clear() error {
	if !p.durable() {
		return nil
	}
	if err := os.Remove(p.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clearing the messages that went out: %w", err)
	}
	return nil
}
