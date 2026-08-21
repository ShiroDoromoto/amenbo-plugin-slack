package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// sendTimeout bounds one POST. An observation hook is never hurried — nothing waits behind it
// but the rest of its own queue — so this is not a deadline for the work, only the point past
// which a connection that will never answer stops holding a process open.
const sendTimeout = 30 * time.Second

// diagnosticLimit is how much of a refusal's body is quoted back. Slack answers a rejected
// post with a short reason (`invalid_payload`, `no_service`), and anything longer than that is
// an error page nobody needs in a log line.
const diagnosticLimit = 200

// sender is the HTTP client, indirected so a test can hand back a refusal without one.
var sender = &http.Client{Timeout: sendTimeout}

// post sends one message to a Slack incoming webhook.
//
// It is sent once. Amenbo does not retry a failed event, and neither does this: two sides
// retrying the same send is how one message becomes three, with nobody able to say why. What
// a failure gets is a line in the execution log saying what Slack refused it with.
func post(webhook, text string) error {
	body, err := json.Marshal(struct {
		Text string `json:"text"`
	}{Text: text})
	if err != nil {
		return fmt.Errorf("building the message: %w", err)
	}
	response, err := sender.Post(webhook, "application/json", bytes.NewReader(body))
	if err != nil {
		// The URL is a secret, so it is named by nothing but the fact that it is the
		// setting: an error that quoted it would put it in the execution log.
		return fmt.Errorf("posting to the webhook: %w", scrub(err, webhook))
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		said, _ := io.ReadAll(io.LimitReader(response.Body, diagnosticLimit))
		return fmt.Errorf("the webhook refused the message: %s %s", response.Status, strings.TrimSpace(string(said)))
	}
	return nil
}

// scrub takes the webhook back out of what a transport error says about it. A URL error names
// the URL it failed on, and this one is the user's secret — a channel anyone reading the log
// could then post to.
func scrub(err error, webhook string) error {
	if webhook == "" || !strings.Contains(err.Error(), webhook) {
		return err
	}
	return fmt.Errorf("%s", strings.ReplaceAll(err.Error(), webhook, "<webhook_url>"))
}
