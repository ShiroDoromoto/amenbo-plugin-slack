package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The manifest as this test asks about it — the fields that have to agree with the code beside it,
// not the whole schema the catalog validates.
type manifest struct {
	Name   string   `json:"name"`
	Events []string `json:"events"`
	Config []field  `json:"config"`
}

type field struct {
	Key      string   `json:"key"`
	Secret   bool     `json:"secret"`
	Required bool     `json:"required"`
	Type     string   `json:"type"`
	Default  string   `json:"default"`
	Options  []option `json:"options"`
}

type option struct {
	Value string `json:"value"`
	Label string `json:"label"`
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

// setting is the declared field under key.
func setting(t *testing.T, key string) field {
	t.Helper()
	for _, declared := range read(t).Config {
		if declared.Key == key {
			return declared
		}
	}
	t.Fatalf("no setting %q is declared", key)
	return field{}
}

// The subscription and the code have to name the same events. An event only the manifest names is a
// launch this plugin has no sentence for; an event only the code names is a sentence never reached,
// since amenbo starts a plugin for what it subscribed to and nothing else.
func TestTheManifestSubscribesToTheWholeCatalog(t *testing.T) {
	events := read(t).Events

	if len(events) != len(catalog) {
		t.Fatalf("the manifest subscribes to %v, the code knows %v", events, catalog)
	}
	for i, event := range events {
		if event != catalog[i] {
			t.Errorf("event %d: the manifest says %q, the code says %q", i, event, catalog[i])
		}
	}
}

// What the user may pick is what the plugin is launched for. An option outside the subscription
// would be a choice that reports nothing, however plainly the form offers it.
func TestTheChoiceOffersExactlyWhatIsSubscribedTo(t *testing.T) {
	events := setting(t, eventsSetting)

	if events.Type != "multi" {
		t.Errorf("picking several needs a multi field, got %q", events.Type)
	}
	if len(events.Options) != len(catalog) {
		t.Fatalf("%d options for %d events", len(events.Options), len(catalog))
	}
	for i, offered := range events.Options {
		if offered.Value != catalog[i] {
			t.Errorf("option %d: the manifest offers %q, the code knows %q", i, offered.Value, catalog[i])
		}
		if offered.Label == "" {
			t.Errorf("option %q has nothing for the user to read", offered.Value)
		}
	}
}

// The default the form is in force with, and the default the code falls back to when no answer
// reaches it at all, are one answer — two spellings of it would report two different things
// depending on which side was asked.
func TestTheDeclaredDefaultIsTheOneTheCodeFallsBackTo(t *testing.T) {
	if got := setting(t, eventsSetting).Default; got != strings.Join(defaultEvents, ",") {
		t.Errorf("the manifest defaults to %q, the code to %q", got, strings.Join(defaultEvents, ","))
	}
}

// The webhook is the other setting, it is a secret — which is what puts it in the environment this
// plugin reads it from — and it is required, since a plugin with nowhere to post does nothing.
func TestTheManifestDeclaresTheWebhookAsARequiredSecret(t *testing.T) {
	if name := read(t).Name; name != "slack" {
		t.Errorf("the plugin's name decides what amenbo runs: %q", name)
	}
	webhook := setting(t, "webhook_url")

	if !webhook.Secret || !webhook.Required {
		t.Errorf("unexpected setting: %+v", webhook)
	}
	// The variable's name follows from the key mechanically, so the one the code reads is the
	// one the declared key becomes — not a second spelling that could drift from it.
	if want := "AMENBO_CONFIG_" + strings.ToUpper(webhook.Key); want != webhookEnv {
		t.Errorf("the setting reaches the plugin as %q, and it is read from %q", want, webhookEnv)
	}
}
