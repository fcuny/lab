package gce

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"golang.org/x/oauth2"
)

// gcloudTokenSource returns short-lived access tokens from the operator's
// gcloud login, so the tools need no Application Default Credentials.
func gcloudTokenSource() oauth2.TokenSource {
	return oauth2.ReuseTokenSource(nil, configHelper{})
}

type configHelper struct{}

func (configHelper) Token() (*oauth2.Token, error) {
	out, err := exec.Command("gcloud", "config", "config-helper", "--format=json").Output()
	if err != nil {
		return nil, fmt.Errorf("gcloud config config-helper: %w (run `gcloud auth login`)", err)
	}
	return parseConfigHelper(out)
}

func parseConfigHelper(out []byte) (*oauth2.Token, error) {
	var v struct {
		Credential struct {
			AccessToken string    `json:"access_token"`
			TokenExpiry time.Time `json:"token_expiry"`
		} `json:"credential"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return nil, fmt.Errorf("parse gcloud config config-helper output: %w", err)
	}
	if v.Credential.AccessToken == "" {
		return nil, fmt.Errorf("gcloud config config-helper returned no access token (run `gcloud auth login`)")
	}
	return &oauth2.Token{AccessToken: v.Credential.AccessToken, Expiry: v.Credential.TokenExpiry}, nil
}

// SSH runs `gcloud compute ssh` on the instance through IAP, with OS Login.
// The key gcloud registers in the operator's OS Login profile expires after
// 24 hours. args go to gcloud after the instance name.
func (c *Client) SSH(ctx context.Context, name string, stdin io.Reader, stdout, stderr io.Writer, args ...string) error {
	full := append([]string{
		"compute", "ssh", name,
		"--project=" + c.Project, "--zone=" + c.Zone,
		"--tunnel-through-iap", "--ssh-key-expire-after=24h",
	}, args...)
	cmd := exec.CommandContext(ctx, "gcloud", full...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	return cmd.Run()
}

// Interactive runs SSH attached to the terminal.
func (c *Client) Interactive(ctx context.Context, name string, args ...string) error {
	return c.SSH(ctx, name, os.Stdin, os.Stdout, os.Stderr, args...)
}
