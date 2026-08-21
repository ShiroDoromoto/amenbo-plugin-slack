package main

import (
	"encoding/json"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The manifest as this test asks about it — the fields that have to agree with the code and the
// build beside it, not the whole schema the catalog validates. What no test here can see is
// whether the release it quotes is the newest one; that is the release procedure's.
type manifest struct {
	Name     string           `json:"name"`
	Repo     string           `json:"repo"`
	OS       []string         `json:"os"`
	Events   []string         `json:"events"`
	Config   []field          `json:"config"`
	Settings settings         `json:"settings"`
	Assets   map[string]asset `json:"assets"`
}

// settings is what the form raises on this plugin: one check, and the buttons.
type settings struct {
	Check   string   `json:"check"`
	Actions []action `json:"actions"`
}

type action struct {
	Cmd   string `json:"cmd"`
	Label string `json:"label"`
}

type asset struct {
	URL      string `json:"url"`
	Checksum string `json:"checksum"`
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

// makefile reads a variable off the build rather than keeping a second copy of it here: the
// platform list a release bakes from, and the name it bakes under. Each is written in one place,
// and what these tests then catch is the manifest that did not follow it.
func makefile(t *testing.T, name string) []string {
	t.Helper()
	raw, err := os.ReadFile("Makefile")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if rest, found := strings.CutPrefix(line, name+" :="); found {
			return strings.Fields(rest)
		}
	}
	t.Fatalf("the Makefile no longer declares %s — these tests read it from there", name)
	return nil
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
// since Amenbo starts a plugin for what it subscribed to and nothing else.
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
		t.Errorf("the plugin's name decides what Amenbo runs: %q", name)
	}
	webhook := setting(t, webhookSetting)

	if !webhook.Secret || !webhook.Required {
		t.Errorf("unexpected setting: %+v", webhook)
	}
	// The variable's name follows from the key mechanically, so the one the code reads is the
	// one the declared key becomes — not a second spelling that could drift from it.
	if want := "AMENBO_CONFIG_" + strings.ToUpper(webhook.Key); want != webhookEnv {
		t.Errorf("the setting reaches the plugin as %q, and it is read from %q", want, webhookEnv)
	}
}

// actionLabelLimit is what a button's label may weigh. It is Amenbo's rule, held here because a
// manifest that breaks it is refused at the catalog door rather than on the screen.
const actionLabelLimit = 40

// The calls the form raises and the calls this plugin answers are the same two. Either half
// alone is a failure the user meets rather than a test does: a check the manifest names and the
// code does not leaves the plugin unenablable, and a button raises a call that ends in the usage
// text.
func TestTheCallsTheFormRaisesAreTheOnesTheCodeAnswers(t *testing.T) {
	m := read(t)

	declared := []string{m.Settings.Check}
	for _, button := range m.Settings.Actions {
		declared = append(declared, button.Cmd)
		if button.Label == "" || len(button.Label) > actionLabelLimit {
			t.Errorf("a button's label is 1 to %d bytes, got %q", actionLabelLimit, button.Label)
		}
	}
	for _, line := range declared {
		if _, answered := faces[line]; !answered {
			t.Errorf("the manifest raises %q, and nothing here answers it", line)
		}
	}
	if len(declared) != len(faces) {
		t.Errorf("the manifest raises %v, the code answers %d call(s)", declared, len(faces))
	}
}

// Every platform the release bakes has to be published under its key, and nothing may be published
// that no run bakes: a key with no build behind it is an install that 404s on the machine it was
// offered to. The keys are the platform names themselves here — every Mac gets its own build, so
// none of them is folded into a universal one.
func TestEveryPlatformTheBuildBakesIsPublished(t *testing.T) {
	assets := read(t).Assets
	baked := makefile(t, "PLATFORMS")

	for _, platform := range baked {
		if _, published := assets[platform]; !published {
			t.Errorf("the build bakes %q, the manifest publishes no asset for it", platform)
		}
	}
	if len(assets) != len(baked) {
		t.Errorf("the manifest publishes %d asset(s) for %d baked platform(s)", len(assets), len(baked))
	}
}

// `os` is what Amenbo weighs against the machine before it offers the plugin at all, so it follows
// from the same list: the operating systems the baked platforms name, and no others. Which order
// they are written in says nothing — two platform keys share each name — so it is not read into.
func TestTheDeclaredOSesAreTheOnesTheBuildBakesFor(t *testing.T) {
	want := map[string]bool{}
	for _, platform := range makefile(t, "PLATFORMS") {
		name, _, _ := strings.Cut(platform, "-")
		want[name] = true
	}

	got := map[string]bool{}
	for _, name := range read(t).OS {
		got[name] = true
	}
	if !maps.Equal(got, want) {
		t.Errorf("the manifest declares %v, the build bakes for %v",
			slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(want)))
	}
}

// One release, quoted the same way by every key. The tag is written twice in each url — once in the
// path and once in the filename — and every asset has to agree with every other, since a single line
// left behind at the previous release serves that one platform an old binary whose digest still
// checks out.
//
// The digest itself is only shaped here. Whether it is the digest of the file it names is for the
// release procedure to settle, against the bytes it downloaded from the release.
func TestEveryAssetQuotesOneRelease(t *testing.T) {
	m := read(t)
	bin := makefile(t, "BIN")
	if len(bin) != 1 {
		t.Fatalf("the Makefile bakes under %v, and an asset is named after one binary", bin)
	}
	url := regexp.MustCompile(`^https://github\.com/(.+)/releases/download/(v\d+)/` +
		regexp.QuoteMeta(bin[0]) + `-(v\d+)-([a-z0-9-]+)\.tar\.gz$`)
	digest := regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

	tags := map[string]string{}
	for _, platform := range makefile(t, "PLATFORMS") {
		published, ok := m.Assets[platform]
		if !ok {
			continue // already reported, by the test that asks for the key at all
		}
		parts := url.FindStringSubmatch(published.URL)
		if parts == nil {
			t.Errorf("%s: %q is not a release asset of this repository", platform, published.URL)
			continue
		}
		repo, inPath, inName, named := parts[1], parts[2], parts[3], parts[4]
		if repo != m.Repo {
			t.Errorf("%s: the url names %q, the manifest names %q", platform, repo, m.Repo)
		}
		if inPath != inName {
			t.Errorf("%s: the url is under %s and the file says %s", platform, inPath, inName)
		}
		if named != platform {
			t.Errorf("%s: the url points at the %s build", platform, named)
		}
		if !digest.MatchString(published.Checksum) {
			t.Errorf("%s: %q is not a sha256 digest", platform, published.Checksum)
		}
		tags[inPath] = platform
	}

	if len(tags) > 1 {
		t.Errorf("the assets are spread over %d releases: %v", len(tags), tags)
	}
}
