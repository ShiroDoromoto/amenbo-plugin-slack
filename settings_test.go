package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// verdictOf runs the check with the webhook set to raw, and reads back what it wrote.
func verdictOf(t *testing.T, raw string) verdict {
	t.Helper()
	t.Setenv(webhookEnv, raw)
	stdout, _ := capture(t)

	if err := configCheck(); err != nil {
		t.Fatalf("a check always has an answer: %v", err)
	}
	var answer verdict
	if err := json.Unmarshal(stdout.Bytes(), &answer); err != nil {
		t.Fatalf("the verdict has to be readable JSON, got %q: %v", stdout, err)
	}
	return answer
}

// shapedLikeAWebhook is a value that passes: the host, the prefix, three parts. Its parts are
// spelled as words rather than as a real webhook's letters and digits on purpose — a fixture with
// the shape of a live credential is one a secret scanner stops a push over, and this repository is
// public.
const shapedLikeAWebhook = "https://hooks.slack.com/services/T-example/B-example/example"

// A URL with the shape of a Slack incoming webhook passes, and the sentence that comes back says
// only what was looked at — nothing here has posted anything.
func TestTheCheckPassesAWebhookShapedURL(t *testing.T) {
	answer := verdictOf(t, shapedLikeAWebhook)

	if !answer.OK || answer.V != verdictVersion {
		t.Errorf("unexpected verdict: %+v", answer)
	}
	if len(answer.Fields) != 0 {
		t.Errorf("a verdict that passed has nothing to say about a box: %+v", answer.Fields)
	}
}

// The three ways a pasted value is not a webhook, each landing on the box it is about.
func TestTheCheckNamesWhatIsNotAWebhook(t *testing.T) {
	for name, raw := range map[string]string{
		"nothing":       "",
		"not https":     "http://hooks.slack.com/services/T0/B0/xxxx",
		"another host":  "https://example.com/services/T0/B0/xxxx",
		"a short path":  "https://hooks.slack.com/services/T0/B0",
		"another path":  "https://hooks.slack.com/api/chat.postMessage",
		"an empty part": "https://hooks.slack.com/services/T0//xxxx",
	} {
		answer := verdictOf(t, raw)

		if answer.OK {
			t.Errorf("%s should not pass as a webhook: %q", name, raw)
		}
		if answer.Fields[webhookSetting] == "" {
			t.Errorf("%s: the sentence belongs on the box it is about: %+v", name, answer)
		}
	}
}

// A verdict is drawn on a screen and travels through amenbo to get there, so what it says about a
// secret is never the secret. Neither is the whole answer allowed to grow past what amenbo reads:
// a sentence over 200 bytes, or carrying a control character, is thrown away entire — and an
// answer thrown away is a plugin that stays disabled.
func TestAVerdictQuotesNothingItWasGiven(t *testing.T) {
	const secret = "https://hooks.slack.com/api/T0PSECRET0/B0PSECRET0/wxyz"

	answer := verdictOf(t, secret)

	for key, said := range answer.Fields {
		if strings.Contains(said, secret) || strings.Contains(said, "T0PSECRET0") {
			t.Errorf("%s quotes what was pasted: %q", key, said)
		}
	}
	for _, said := range append(sentences(answer), verdictOf(t, shapedLikeAWebhook).Message) {
		if len(said) > 200 {
			t.Errorf("a sentence amenbo will not read is one nobody sees: %d bytes, %q", len(said), said)
		}
		if strings.ContainsFunc(said, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
			t.Errorf("a sentence carrying a control character is thrown away whole: %q", said)
		}
	}
}

// sentences is everything in a verdict that amenbo draws.
func sentences(answer verdict) []string {
	said := []string{answer.Message}
	for _, one := range answer.Fields {
		said = append(said, one)
	}
	return said
}

// A check is not how "the URL is wrong" is said. It ends zero whatever it found, because a
// non-zero exit means it did not answer at all — which amenbo reads as never having checked, and
// which leaves the plugin disabled for a reason that has nothing to do with the value.
func TestACheckWithABadValueStillEndsWell(t *testing.T) {
	t.Setenv(webhookEnv, "not a webhook")
	capture(t)

	if err := configCheck(); err != nil {
		t.Errorf("a check that has an answer has not failed: %v", err)
	}
}

// The test message is a line in a channel, so it is written in the language the store is read
// in — not the one the author happens to write in.
func TestTheTestMessageIsSaidInTheStoresLanguage(t *testing.T) {
	if ja, en := testLine("ja"), testLine("en"); ja == en || ja == "" {
		t.Errorf("a language with a row of its own says it in that row: %q vs %q", ja, en)
	}
	if unheard := testLine("xx"); unheard != testLine(fallbackLanguage) {
		t.Errorf("a language with no row falls back to English, got %q", unheard)
	}
}

// Pressing the button posts one message, headed by the project the press was made in — which is
// the whole of what the press is asking: does a line written here land in that channel.
func TestTheTestPostsOneHeadedMessage(t *testing.T) {
	posted := slackStands(t)
	namesTheProject(t, "amenbo-plugin-slack")
	t.Setenv(reachEnv, "AMB-P-9")
	_, stderr := capture(t)

	if err := configTest(); err != nil {
		t.Fatal(err)
	}

	if len(*posted) != 1 || (*posted)[0] != "*amenbo-plugin-slack*\n"+testLine("en") {
		t.Errorf("unexpected message: %q", *posted)
	}
	if stderr.Len() == 0 {
		t.Error("the press should leave the one line the settings form draws")
	}
}

// A store that will not answer costs the message its heading and its language, and nothing else.
// The exit code here means *did it arrive*, so ending non-zero over a read would say the webhook
// is broken while the message sits in the channel.
func TestTheTestStillPostsWhenNothingCanBeReadBack(t *testing.T) {
	posted := slackStands(t)
	refusesToRead(t, "out_of_reach")
	t.Setenv(reachEnv, "AMB-P-9")
	capture(t)

	if err := configTest(); err != nil {
		t.Errorf("the message arrived, so the press worked: %v", err)
	}
	if len(*posted) != 1 || (*posted)[0] != testLine(fallbackLanguage) {
		t.Errorf("unexpected message: %q", *posted)
	}
}

// Pressing the button with nothing to post to is refused where it can be said plainly, rather
// than by handing an empty URL to the transport.
func TestTheTestRefusesWithNoWebhook(t *testing.T) {
	t.Setenv(webhookEnv, "")
	capture(t)

	err := configTest()

	if err == nil || !strings.Contains(err.Error(), webhookSetting) {
		t.Errorf("the refusal should say which setting is empty, got %v", err)
	}
}
