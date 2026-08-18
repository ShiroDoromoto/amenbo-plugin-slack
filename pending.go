package main

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
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
//   - **What is held is bounded.** A send that keeps failing — a webhook revoked, say — is a flush
//     that never empties what it carries, so the hold is capped and the oldest lines fall off it.

// pendingFile holds the messages taken in but not yet sent, one JSON object per line — JSON because a
// title is the user's text and could carry anything, newlines included, and a line that broke in two
// would arrive as two messages. An object rather than the bare string it used to be, because a line
// also has to remember which part of the message it belongs in (see [partOf]); a bare string is
// still read, being what a build before the parts left behind, and lands among what the AI did.
const pendingFile = "pending-messages.log"

// The parts a message is laid out in, and the order they are laid out in. What the AI did comes
// first and reads as it happened; the due dates follow, each kind kept together — the ones already
// standing before the ones that arrive tomorrow, urgent first.
//
// Keeping the two kinds apart is what makes the second one readable at all. A tick can name a dozen
// tasks at once, and a message that interleaved "is due" with "is due tomorrow" would ask the reader
// to sort it themselves, line by line, to find out what is actually late.
const (
	partActs        = ""
	partDue         = "due"
	partDueTomorrow = "due-tomorrow"
)

// partOrder is that order. A part not in it — a file written by a build that knows one this one does
// not — is read as [partActs] rather than dropped, a line in the wrong part still saying what
// happened.
var partOrder = []string{partActs, partDue, partDueTomorrow}

// partOf says which part an event's line belongs in.
func partOf(event string) string {
	switch event {
	case eventTaskDue:
		return partDue
	case eventTaskDueTomorrow:
		return partDueTomorrow
	}
	return partActs
}

// heldAtMost is how many messages are kept waiting. It is [remembered], because this is the same
// kind of state kept in the same place for the same reason: what a run has to carry over to the next
// one has to stay bounded, or a fault nobody is watching turns it into a file that grows for good.
//
// Nothing bounds it otherwise. A flush that Slack refused leaves its lines owed and the next flush
// carries them, which is what makes a refusal late rather than lost — but a webhook that has been
// revoked refuses every flush, and then the batch only ever grows. It grows into a message longer
// than Slack will take, and from there the send fails on its own length: fixing the webhook would no
// longer fix the channel. Dropping the oldest lines is what that costs, and it is the cheaper loss.
const heldAtMost = remembered

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

// line is one message taken in: what it says, and which part of the message it belongs in. The
// keys are short because every held line carries them.
type line struct {
	Part string `json:"p,omitempty"`
	Text string `json:"t"`
}

// pending is what has been taken in and not yet sent: whatever earlier runs held, in the order they
// held it.
type pending struct {
	// path is where the messages are kept, empty when there is nowhere to keep them.
	path     string
	messages []line
}

// held reads what is owed. A file that cannot be read comes back empty rather than refusing the run —
// the cost is a message that stays unsent, and refusing would add one that never went out at all.
func held() pending {
	path := statePath(pendingFile)
	if path == "" {
		return pending{}
	}
	owed := pending{path: path}
	for _, held := range readLines(path) {
		message, err := readHeld(held)
		if err != nil {
			// A line this plugin cannot read is one it cannot send; dropping it loses one
			// message, while stopping here would lose every message behind it too.
			logf("slack: skipping a held message that will not parse: %v", err)
			continue
		}
		owed.messages = append(owed.messages, message)
	}
	return owed
}

// readHeld reads one held line, in either of the two shapes the file has had: an object saying which
// part it belongs in, and the bare string a build before the parts wrote. The older shape is read
// rather than dropped — what it holds is a message someone is still owed, and the part it lacks is
// the one that came first anyway.
func readHeld(held string) (line, error) {
	if strings.HasPrefix(strings.TrimSpace(held), "\"") {
		var text string
		if err := json.Unmarshal([]byte(held), &text); err != nil {
			return line{}, err
		}
		return line{Text: text}, nil
	}
	var message line
	if err := json.Unmarshal([]byte(held), &message); err != nil {
		return line{}, err
	}
	return message, nil
}

// durable says whether what is held will survive this run. Where there is nowhere to write, holding
// a message would mean carrying it in a process that is about to end — so nothing is held back at
// all, and every event is its own message.
func (p pending) durable() bool {
	return p.path != ""
}

// add puts one message at the end of what is owed, in the part of the message it belongs in.
func (p *pending) add(part, message string) {
	p.messages = append(p.messages, line{Part: part, Text: message})
}

// loosen gives up keeping what is owed: nothing will be held back, so every message goes out as it
// arrives. What it costs is the batching, which is the right thing to lose — a message held in a
// process about to end is a message nobody ever gets.
func (p *pending) loosen() {
	p.path = ""
}

// bound drops what no longer fits, oldest first, and says so on stderr so the run that lost it can be
// read back in `amenbo plugin log slack`.
//
// It happens where what is owed is written down, which is the one moment every held line passes
// through, and it is no business of the send's: whether the last message got through says nothing
// about how much has piled up behind it. What is dropped goes from this run's batch as well as from
// the file, so a flush carries what is held and not what was.
//
// Saying it on stderr is what makes the loss visible at all. Putting it in the message instead would
// only reach the channel once a send finally got through — and while nothing does is exactly when
// lines are being dropped.
func (p *pending) bound() {
	if len(p.messages) <= heldAtMost {
		return
	}
	dropped := len(p.messages) - heldAtMost
	p.messages = p.messages[dropped:]
	logf("slack: dropped %d of the messages waiting to be sent, oldest first — %d is as many as this plugin holds", dropped, heldAtMost)
}

// save writes what is owed down, so a run that does not come back has not swallowed it — keeping no
// more than [heldAtMost] of it.
func (p *pending) save() error {
	if !p.durable() {
		return nil
	}
	p.bound()
	lines := make([]string, 0, len(p.messages))
	for _, message := range p.messages {
		held, err := json.Marshal(message)
		if err != nil {
			return fmt.Errorf("holding a message for the rest of the queue: %w", err)
		}
		lines = append(lines, string(held))
	}
	if err := writeWhole(p.path, lines); err != nil {
		return fmt.Errorf("holding a message for the rest of the queue: %w", err)
	}
	return nil
}

// text is everything owed as one message: one line each, laid out in parts and separated by a blank
// line. Within a part the lines are in the order they were taken in, which for what the AI did is the
// order it happened in.
func (p pending) text() string {
	var blocks []string
	for _, part := range partOrder {
		var lines []string
		for _, message := range p.messages {
			if message.part() == part {
				lines = append(lines, message.Text)
			}
		}
		if len(lines) > 0 {
			blocks = append(blocks, strings.Join(lines, "\n"))
		}
	}
	return strings.Join(blocks, "\n\n")
}

// part is where a line is laid out, reading a part this build does not know as the first one so that
// the line is still in the message.
func (l line) part() string {
	if slices.Contains(partOrder, l.Part) {
		return l.Part
	}
	return partActs
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
