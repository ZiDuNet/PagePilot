package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestConfigCommandsEmitJSONAndNeverExposeShortToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	oldJSON := flagJSON
	flagJSON = true
	defer func() { flagJSON = oldJSON }()

	set := cmdConfig()
	set.SetArgs([]string{"set", "token", "short"})
	setOutput := captureStdout(t, func() {
		if err := set.Execute(); err != nil {
			t.Fatalf("config set: %v", err)
		}
	})
	var setPayload map[string]any
	if err := json.Unmarshal([]byte(setOutput), &setPayload); err != nil {
		t.Fatalf("config set output = %q: %v", setOutput, err)
	}
	if setPayload["success"] != true || setPayload["tokenSaved"] != true {
		t.Fatalf("config set payload = %#v", setPayload)
	}

	get := cmdConfig()
	get.SetArgs([]string{"get", "token"})
	getOutput := captureStdout(t, func() {
		if err := get.Execute(); err != nil {
			t.Fatalf("config get: %v", err)
		}
	})
	var getPayload map[string]any
	if err := json.Unmarshal([]byte(getOutput), &getPayload); err != nil {
		t.Fatalf("config get output = %q: %v", getOutput, err)
	}
	if got := getPayload["value"]; got != "***" {
		t.Fatalf("config get token = %#v, want masked value", got)
	}
	if strings.Contains(getOutput, "short") {
		t.Fatalf("config get leaked token: %q", getOutput)
	}

	show := cmdConfig()
	show.SetArgs([]string{"show"})
	showOutput := captureStdout(t, func() {
		if err := show.Execute(); err != nil {
			t.Fatalf("config show: %v", err)
		}
	})
	if strings.Contains(showOutput, "short") || !strings.Contains(showOutput, "***") {
		t.Fatalf("config show output = %q; want masked token", showOutput)
	}
}

func TestTokenSaveEmitsJSONWithoutTokenMaterial(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	oldServer, oldToken, oldJSON := flagServer, flagToken, flagJSON
	flagServer, flagToken, flagJSON = "https://pagepilot.example", "", true
	defer func() { flagServer, flagToken, flagJSON = oldServer, oldToken, oldJSON }()

	command := cmdToken()
	command.SetArgs([]string{"save", "plain-token-value"})
	output := captureStdout(t, func() {
		if err := command.Execute(); err != nil {
			t.Fatalf("token save: %v", err)
		}
	})
	var payload map[string]any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("token save output = %q: %v", output, err)
	}
	if payload["success"] != true || payload["tokenSaved"] != true {
		t.Fatalf("token save payload = %#v", payload)
	}
	if strings.Contains(output, "plain-token-value") {
		t.Fatalf("token save output leaked token: %q", output)
	}
	if _, err := os.Stat(configPath()); err != nil {
		t.Fatalf("saved config missing: %v", err)
	}
}
