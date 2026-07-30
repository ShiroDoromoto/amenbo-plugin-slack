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

// taskShow reads back the two things a message needs about a task: the ref a reader can look
// it up by, and its title. A payload carries neither — it names the record and stops there.
//
// A failure here is returned rather than raised: what it costs is the title, not the message
// (see hook). The refusals worth expecting are a window that does not cover this task and a
// store the caller's amenbo cannot open, and both say so on stderr, which is carried into the
// error so the execution log holds the reason and not just the fact.
func taskShow(id int64) (ref, title string, err error) {
	args := append([]string{"task", "show", strconv.FormatInt(id, 10), "--json"}, actorFlag...)
	raw, err := runAmenbo(args...)
	if err != nil {
		return "", "", err
	}
	var task struct {
		Ref   string `json:"ref"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal(raw, &task); err != nil {
		return "", "", fmt.Errorf("reading back task %d: %w", id, err)
	}
	return task.Ref, task.Title, nil
}
