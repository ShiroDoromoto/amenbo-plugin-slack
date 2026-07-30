package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// capture redirects the plugin's two channels into buffers for the length of one test.
func capture(t *testing.T) (stdout, stderr *bytes.Buffer) {
	t.Helper()
	stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
	previousOut, previousErr := out, errOut
	out, errOut = stdout, stderr
	t.Cleanup(func() { out, errOut = previousOut, previousErr })
	return stdout, stderr
}

// stdinWith writes doc to a temp file and opens it, standing in for the pipe amenbo feeds the
// plugin.
func stdinWith(t *testing.T, doc string) *os.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdin.json")
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

// The document amenbo writes is read whole, including the fields this plugin acts on.
func TestReadInputTakesTheEventOffStdin(t *testing.T) {
	in := readInput(stdinWith(t, `{"v":1,"event":"task.done","id":42,"actor":"ai","at":"2026-07-22T09:00:00Z"}`))

	if in.V != 1 || in.Event != eventTaskDone || in.ID != 42 || in.Actor != actorAI {
		t.Errorf("the event should arrive whole, got %+v", in)
	}
}

// A field this plugin has never heard of is not a reason to stop reading the rest: the
// contract grows by addition.
func TestReadInputIgnoresFieldsItDoesNotKnow(t *testing.T) {
	in := readInput(stdinWith(t, `{"v":1,"event":"task.done","id":7,"actor":"ai","something_new":{"a":1}}`))

	if in.ID != 7 {
		t.Errorf("the known fields should still be read, got %+v", in)
	}
}

// A document that will not parse is reported and dropped — there is no event left to report,
// and nobody is waiting on the answer.
func TestReadInputDropsADocumentThatWillNotParse(t *testing.T) {
	_, stderr := capture(t)

	in := readInput(stdinWith(t, `{not json`))

	if !nothingRead(in) {
		t.Errorf("nothing should be read out of it, got %+v", in)
	}
	if !strings.Contains(stderr.String(), "will not parse") {
		t.Errorf("the drop should say why: %q", stderr)
	}
}

// Nothing on stdin is the ordinary shape of a hand run, not an error.
func TestReadInputTakesAnEmptyStdin(t *testing.T) {
	if in := readInput(stdinWith(t, "")); !nothingRead(in) {
		t.Errorf("an empty document is no event, got %+v", in)
	}
}

// nothingRead is what a dropped or absent document reads as. Spelled out field by field, since a
// payload carrying maps is not a value Go will compare for us.
func nothingRead(in input) bool {
	return in.V == 0 && in.Event == "" && in.ID == 0 && in.Actor == "" && in.New == "" &&
		in.Record == nil && in.Parent == nil && in.Config == nil
}

// The usage text names the setting and the switch, since a plugin that reported nothing is
// almost always one of the two.
func TestUsageNamesTheSettingAndTheSwitch(t *testing.T) {
	_, stderr := capture(t)

	usage()

	for _, want := range []string{"webhook_url", "plugin enable slack", "plugin log slack"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("the usage should mention %q: %q", want, stderr)
		}
	}
}
