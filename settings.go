package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strings"
)

// This file is the settings form's side of the plugin. The manifest names two calls under
// `settings` and they are answered here, through the ordinary command face — a subcommand and
// its arguments, reachable from a terminal the same way the form reaches it:
//
//	amenbo plugin run slack config check
//	amenbo plugin run slack config test
//
// They divide by what they may cost. `check` runs unasked — when the plugin is enabled, and
// after every save while it is — and is held to two seconds, so it reads the value and stops
// there: a check that posted would put a line in the channel every time the user saved. `test`
// is a button, pressed on purpose, and is the one that posts, because whether a webhook still
// works is a thing nothing but a real message can answer.

// The command face, spelled once. The manifest declares the same two lines and a test holds
// the two together — a call the form raises that nothing here answers is a button that fails
// on every press.
const (
	commandConfig = "config"
	commandCheck  = "check"
	commandTest   = "test"
)

// faces is every call this plugin answers on purpose, keyed by the line the manifest declares.
// The observation hook is not among them: it is fired with no arguments and is not a call.
var faces = map[string]func() error{
	commandConfig + " " + commandCheck: configCheck,
	commandConfig + " " + commandTest:  configTest,
}

// webhookSetting is the key the webhook is stored under — the name the user's box has on the
// settings form, and the one a verdict has to speak about it under for the sentence to land
// beside the right box.
const webhookSetting = "webhook_url"

// verdictVersion is the version of the answer a check writes. It is a number of its own rather
// than the payload's: what amenbo writes to a plugin and what a plugin writes back are two
// contracts, and they are free to move apart.
const verdictVersion = 1

// verdict is what a check writes on stdout. `ok` is the whole of the gate — a check that does
// not say yes leaves the plugin disabled — and the sentences are the settings form's alone:
// the CLI's refusal names the keys and none of this text, and `amenbo agent --json` carries no
// part of it.
type verdict struct {
	V       int               `json:"v"`
	OK      bool              `json:"ok"`
	Message string            `json:"message,omitempty"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// Where a Slack incoming webhook lives. Slack hands the whole URL over in one piece and the
// user pastes it, so what can go wrong is pasting something else — the workspace's home page,
// a bot token's URL, half of one. All three are visible in the shape.
const (
	webhookScheme = "https"
	webhookHost   = "hooks.slack.com"
	webhookPrefix = "/services/"
	webhookParts  = 3
)

// checkedShape is what a check that found nothing wrong says about the settings as a whole. It
// is careful about what it claims: the shape is all this call looked at, and a webhook that was
// revoked yesterday still has it.
const checkedShape = "The URL has the shape of a Slack incoming webhook. Whether it still works is what a test message answers."

// configCheck answers whether the settings are usable, at the two moments the answer can still
// be acted on.
//
// It ends zero whatever it found. A verdict of `ok: false` is this call having run and having
// an answer; a non-zero exit is this call not having answered at all, which amenbo reads as
// *not checked* — the same as a crash. Saying "the URL is wrong" by failing would be saying
// something else entirely.
func configCheck() error {
	answer := verdict{V: verdictVersion, OK: true, Message: checkedShape}
	if wrong := checkWebhook(os.Getenv(webhookEnv)); wrong != "" {
		answer = verdict{V: verdictVersion, Fields: map[string]string{webhookSetting: wrong}}
	}
	written, err := json.Marshal(answer)
	if err != nil {
		return fmt.Errorf("writing the verdict: %w", err)
	}
	fmt.Fprintln(out, string(written))
	return nil
}

// checkWebhook says what is wrong with a webhook URL, in the one sentence the box gets, or
// nothing when it looks like one.
//
// What it does not do is quote the value back. The sentence is drawn on the settings form,
// which is the one place the URL is already visible, but it is also the author's text passing
// through amenbo — and a secret that travels in it is a secret in a place nobody thought to
// guard. So every sentence here describes the shape and names none of what was pasted.
func checkWebhook(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "There is no webhook here to post to."
	}
	address, err := url.Parse(raw)
	if err != nil {
		return "This is not a URL."
	}
	if address.Scheme != webhookScheme {
		return "A Slack incoming webhook is an " + webhookScheme + " URL, and this is not."
	}
	if address.Host != webhookHost {
		return "A Slack incoming webhook is at " + webhookHost + ", and this URL is not."
	}
	rest, found := strings.CutPrefix(address.Path, webhookPrefix)
	parts := strings.Split(rest, "/")
	if !found || len(parts) != webhookParts || slices.Contains(parts, "") {
		return fmt.Sprintf("A Slack incoming webhook's path is %s and %d parts after it, like /T…/B…/… — this one is not.", webhookPrefix, webhookParts)
	}
	return ""
}

// configTest posts one message, which is the only way to find out whether the webhook is still
// good for one. It is what the button on the settings form raises.
//
// The reads behind the message are not what the press is asking about, so neither one failing
// ends the run: a store that will not answer costs the line its language and the message its
// heading, exactly as it does on the observation hook. Here, though, the exit code is read as
// *did it arrive*, and a run that ended non-zero over a heading would be telling the user their
// webhook is broken while the message sits in the channel in front of them.
func configTest() error {
	webhook := os.Getenv(webhookEnv)
	if webhook == "" {
		return fmt.Errorf("no webhook to post to — set it with 'amenbo plugin config set slack %s <url>'", webhookSetting)
	}
	how, _ := readPreferences()
	head, _ := heading()
	if err := post(webhook, head+testLine(how.language)); err != nil {
		return err
	}
	// One line, because one line is what the settings form draws.
	logf("the test message went out — look for it in the channel this project reports to")
	return nil
}
