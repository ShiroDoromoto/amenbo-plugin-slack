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
// "AI finished AMB-T-42" twice reads as two events, and nothing in it explains which. So the memory
// of what has been taken in lives here, and it guards the taking.
//
// **Taken in, not sent.** A message may be held back for the rest of the queue (see pending.go), so
// what stops a replay has to be the earlier moment — the point where this plugin took charge of the
// event — or a replay arriving while its message is still waiting would be held a second time and
// the batch would say the same thing twice.
//
// **What it does not guard is the user doing the same thing twice.** Two writes are two events with
// two moments, so their keys differ and both are reported. What a replay repeats is one moment.

// takenFile holds the keys of what has been taken in, one per line, oldest first.
const takenFile = "taken-events.log"

// remembered is how many keys are kept. A replay follows the run it repeats within one queue's
// lifetime, so the window only has to outlast a burst — fifty events queued by one project
// deletion, say — not the history of the store. Keeping a bounded tail is also what stops the file
// from growing without end.
const remembered = 200

// statePath is where this plugin keeps what it has to remember between runs: in its own installed
// directory, so `plugin uninstall` takes it away with everything else the plugin left behind, and a
// second store keeps its own. Empty when no store was named — nothing launched this as a plugin, so
// there is no home to write under.
func statePath(name string) string {
	home := strings.TrimSpace(os.Getenv(storeEnv))
	if home == "" {
		return ""
	}
	return filepath.Join(home, "plugins", pluginName, name)
}

// writeWhole replaces a state file with these lines.
//
// Written to a temp file and renamed over the old one: a run cut off mid-write would otherwise leave
// a half line behind, and the next run would read it as something it is not.
func writeWhole(path string, lines []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body := ""
	if len(lines) > 0 {
		body = strings.Join(lines, "\n") + "\n"
	}
	temp := path + ".new"
	if err := os.WriteFile(temp, []byte(body), 0o644); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

// readLines reads a state file's non-empty lines; a file that is not there yet is no lines, which is
// the ordinary shape of a first run rather than a fault.
func readLines(path string) []string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var lines []string
	for _, line := range strings.Split(string(raw), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// takenKey identifies one event: what happened, to which record, at which moment. The moment is what
// separates a replay from a repeat — amenbo hands over the same `at` when it delivers an event
// again, and a different one when the user acts again.
func takenKey(in input) string {
	return in.Event + " " + strconv.FormatInt(in.ID, 10) + " " + in.At
}

// memory is the tail of what has been taken in, as read at the start of a run.
type memory struct {
	// path is where the tail is kept, empty when there is nowhere to keep it.
	path string
	keys []string
}

// recall reads the memory. Two things can go wrong and neither is worth refusing to report over: no
// store was named, and a file that cannot be read. Both come back as a memory that holds nothing,
// which reports the event and forgets it — a duplicate is a smaller fault than a silence.
func recall() memory {
	path := statePath(takenFile)
	if path == "" {
		return memory{}
	}
	return memory{path: path, keys: readLines(path)}
}

// holds says whether this event has already been taken in.
func (m memory) holds(key string) bool {
	for _, taken := range m.keys {
		if taken == key {
			return true
		}
	}
	return false
}

// add records one key, keeping the last [remembered] of them.
func (m *memory) add(key string) error {
	if m.path == "" {
		return nil
	}
	m.keys = append(m.keys, key)
	if len(m.keys) > remembered {
		m.keys = m.keys[len(m.keys)-remembered:]
	}
	if err := writeWhole(m.path, m.keys); err != nil {
		return fmt.Errorf("keeping the record of what was taken in: %w", err)
	}
	return nil
}
