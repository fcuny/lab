package gce

import (
	"testing"
	"time"
)

func TestParseConfigHelper(t *testing.T) {
	out := []byte(`{"configuration": {}, "credential": {"access_token": "tok", "token_expiry": "2026-11-25T15:03:23Z"}}`)
	tok, err := parseConfigHelper(out)
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "tok" {
		t.Errorf("AccessToken = %q, want %q", tok.AccessToken, "tok")
	}
	if want := time.Date(2026, 11, 25, 15, 3, 23, 0, time.UTC); !tok.Expiry.Equal(want) {
		t.Errorf("Expiry = %v, want %v", tok.Expiry, want)
	}
}

func TestParseConfigHelperErrors(t *testing.T) {
	for _, out := range []string{`not json`, `{"credential": {}}`} {
		if _, err := parseConfigHelper([]byte(out)); err == nil {
			t.Errorf("parseConfigHelper(%q) succeeded, want an error", out)
		}
	}
}
