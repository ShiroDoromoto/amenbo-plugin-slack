package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// manifest is the shape this test asks about — the fields that have to agree with the code
// beside it, not the whole schema the catalog validates.
type manifest struct {
	Name   string   `json:"name"`
	Events []string `json:"events"`
	Config []struct {
		Key      string `json:"key"`
		Secret   bool   `json:"secret"`
		Required bool   `json:"required"`
	} `json:"config"`
}

func read(t *testing.T) manifest {
	t.Helper()
	raw, err := os.ReadFile("dev/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// The subscription and the code have to name the same events. An event only the manifest names
// is a launch that reports nothing; an event only the code names is a sentence never reached,
// since amenbo starts a plugin for what it subscribed to and nothing else.
func TestTheManifestSubscribesToExactlyWhatIsReported(t *testing.T) {
	events := read(t).Events

	if len(events) != len(reported) {
		t.Fatalf("the manifest subscribes to %v, the code reports %v", events, reported)
	}
	for i, event := range events {
		if event != reported[i] {
			t.Errorf("event %d: the manifest says %q, the code says %q", i, event, reported[i])
		}
	}
}

// The webhook is the one setting, it is a secret — which is what puts it in the environment
// this plugin reads it from — and it is required, since a plugin with nowhere to post is a
// plugin that does nothing.
func TestTheManifestDeclaresTheWebhookAsARequiredSecret(t *testing.T) {
	m := read(t)

	if m.Name != "slack" {
		t.Errorf("the plugin's name decides what amenbo runs: %q", m.Name)
	}
	if len(m.Config) != 1 {
		t.Fatalf("there is one setting, got %+v", m.Config)
	}
	if m.Config[0].Key != "webhook_url" || !m.Config[0].Secret || !m.Config[0].Required {
		t.Errorf("unexpected setting: %+v", m.Config[0])
	}
	// The variable's name follows from the key mechanically, so the one the code reads is
	// the one the declared key becomes — not a second spelling that could drift from it.
	if want := "AMENBO_CONFIG_" + strings.ToUpper(m.Config[0].Key); want != webhookEnv {
		t.Errorf("the setting reaches the plugin as %q, and it is read from %q", want, webhookEnv)
	}
}
