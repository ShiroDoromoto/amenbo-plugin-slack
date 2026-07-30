package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The same event can arrive twice. A runner that dies after a plugin finished an event but before
// amenbo took that row off the queue replays it, and amenbo cannot see what happened on the other
// side, so it cannot prevent this — making the second run a no-op is the plugin's job.
//
// Nobody on the receiving end can tell that second message from a real one: a channel showing
// "AI finished AMB-T-42" twice reads as two events, and nothing in it explains which. So the
// memory of what was sent lives here, and the send is what it guards.
//
// **What it does not guard is the user doing the same thing twice.** Two writes are two events with
// two moments, so their keys differ and both are reported. What a replay repeats is one moment.

// sentFile holds the keys of what has been sent, one per line, oldest first. It lives beside the
// binary in the plugin's own installed directory, so `plugin uninstall` takes it away with
// everything else the plugin left behind, and a second store keeps its own.
const sentFile = "sent-events.log"

// remembered is how many keys are kept. A replay follows the run it repeats within one queue's
// lifetime, so the window only has to outlast a burst — fifty events queued by one project
// deletion, say — not the history of the store. Keeping a bounded tail is also what stops the file
// from growing without end.
const remembered = 200

// sentKey identifies one event: what happened, to which record, at which moment. The moment is what
// separates a replay from a repeat — amenbo hands over the same `at` when it delivers an event
// again, and a different one when the user acts again.
func sentKey(in input) string {
	return in.Event + " " + strconv.FormatInt(in.ID, 10) + " " + in.At
}

// memory is the tail of what has been sent, as read at the start of a run.
type memory struct {
	// path is where the tail is kept, empty when there is nowhere to keep it.
	path string
	keys []string
}

// recall reads the memory. Two things can go wrong and neither is worth refusing to report over: no
// store was named (nothing launched this as a plugin, so there is no home to write under), and a
// file that cannot be read. Both come back as a memory that holds nothing, which sends the message
// and forgets it — a duplicate is a smaller fault than a silence.
func recall() memory {
	home := strings.TrimSpace(os.Getenv(storeEnv))
	if home == "" {
		return memory{}
	}
	path := filepath.Join(home, "plugins", pluginName, sentFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		// A first run has no file yet, which is the ordinary case rather than a fault.
		return memory{path: path}
	}
	var keys []string
	for _, line := range strings.Split(string(raw), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			keys = append(keys, line)
		}
	}
	return memory{path: path, keys: keys}
}

// holds says whether this event has already been sent.
func (m memory) holds(key string) bool {
	for _, sent := range m.keys {
		if sent == key {
			return true
		}
	}
	return false
}

// add records one key, keeping the last [remembered] of them.
//
// Written whole to a temp file and renamed over the old one: a run cut off mid-write would
// otherwise leave a half line that reads as a key nobody sent, and the next run would trust it.
func (m *memory) add(key string) error {
	if m.path == "" {
		return nil
	}
	m.keys = append(m.keys, key)
	if len(m.keys) > remembered {
		m.keys = m.keys[len(m.keys)-remembered:]
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return fmt.Errorf("keeping the record of what was sent: %w", err)
	}
	temp := m.path + ".new"
	if err := os.WriteFile(temp, []byte(strings.Join(m.keys, "\n")+"\n"), 0o644); err != nil {
		return fmt.Errorf("keeping the record of what was sent: %w", err)
	}
	if err := os.Rename(temp, m.path); err != nil {
		return fmt.Errorf("keeping the record of what was sent: %w", err)
	}
	return nil
}
