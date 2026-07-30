package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The message goes out as the one field an incoming webhook reads, declared as JSON.
func TestPostSendsTheTextAsJSON(t *testing.T) {
	var contentType string
	var body struct{ Text string }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decoding the message: %v", err)
		}
		fmt.Fprint(w, "ok")
	}))
	defer server.Close()

	if err := post(server.URL, "AI finished AMB-T-42 — Ship the thing"); err != nil {
		t.Fatal(err)
	}

	if contentType != "application/json" {
		t.Errorf("unexpected content type: %q", contentType)
	}
	if body.Text != "AI finished AMB-T-42 — Ship the thing" {
		t.Errorf("unexpected message: %q", body.Text)
	}
}

// Anything but a 2xx is a message that did not arrive, and Slack's own reason for it is the
// one thing a log line here is worth.
func TestPostCarriesBackWhatTheWebhookRefusedItWith(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "invalid_payload")
	}))
	defer server.Close()

	err := post(server.URL, "AI finished AMB-T-42")

	if err == nil || !strings.Contains(err.Error(), "invalid_payload") || !strings.Contains(err.Error(), "400") {
		t.Errorf("the status and the reason should both be there, got %v", err)
	}
}

// A webhook URL is a secret. A transport error names the URL it failed on, so what reaches the
// execution log has it taken back out — a log a channel can be posted to from is a leak.
func TestPostKeepsTheWebhookOutOfWhatItSays(t *testing.T) {
	webhook := "http://127.0.0.1:1/services/T000/B000/xoxb-not-a-real-token"

	err := post(webhook, "AI finished AMB-T-42")

	if err == nil {
		t.Fatal("a webhook nothing is listening on should fail")
	}
	if strings.Contains(err.Error(), "xoxb-not-a-real-token") {
		t.Errorf("the secret should not be in the diagnostic: %v", err)
	}
	if !strings.Contains(err.Error(), "<webhook_url>") {
		t.Errorf("what was taken out should still be named: %v", err)
	}
}
