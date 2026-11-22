package main

import (
	"strings"
	"testing"
	"time"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestCommandExecutionIsOffByDefault(t *testing.T) {
	for _, value := range []string{"", "false", "0", "no", " FALSE "} {
		s, err := loadCommandSettings(env(map[string]string{"GATEWAY_COMMANDS_ENABLED": value}))
		if err != nil || s.Enabled {
			t.Fatalf("%q: enabled=%v err=%v", value, s.Enabled, err)
		}
	}
}

func TestCommandSettingsDefaultsAndOverrides(t *testing.T) {
	s, err := loadCommandSettings(env(map[string]string{"GATEWAY_COMMANDS_ENABLED": "true"}))
	if err != nil || !s.Enabled || s.StateDir != defaultStateDir || s.PollInterval != 10*time.Second {
		t.Fatalf("defaults = %+v err=%v", s, err)
	}
	s, err = loadCommandSettings(env(map[string]string{
		"GATEWAY_COMMANDS_ENABLED":      "yes",
		"GATEWAY_STATE_DIR":             "/srv/mtandao/../state/",
		"GATEWAY_COMMAND_POLL_INTERVAL": "30",
		"GATEWAY_SITE_ID":               "AAAAAAAA-1111-2222-3333-444444444444",
		"GATEWAY_ROUTER_ID":             "bbbbbbbb-1111-2222-3333-444444444444",
	}))
	if err != nil || s.StateDir != "/srv/state" || s.PollInterval != 30*time.Second ||
		s.SiteID != "aaaaaaaa-1111-2222-3333-444444444444" || s.RouterID != "bbbbbbbb-1111-2222-3333-444444444444" {
		t.Fatalf("overrides = %+v err=%v", s, err)
	}
}

func TestCommandSettingsRejectInvalidValues(t *testing.T) {
	cases := map[string]map[string]string{
		"bad flag":      {"GATEWAY_COMMANDS_ENABLED": "maybe"},
		"relative dir":  {"GATEWAY_COMMANDS_ENABLED": "true", "GATEWAY_STATE_DIR": "state"},
		"interval low":  {"GATEWAY_COMMANDS_ENABLED": "true", "GATEWAY_COMMAND_POLL_INTERVAL": "1"},
		"interval high": {"GATEWAY_COMMANDS_ENABLED": "true", "GATEWAY_COMMAND_POLL_INTERVAL": "301"},
		"interval text": {"GATEWAY_COMMANDS_ENABLED": "true", "GATEWAY_COMMAND_POLL_INTERVAL": "soon"},
		"bad site":      {"GATEWAY_COMMANDS_ENABLED": "true", "GATEWAY_SITE_ID": "not-a-uuid"},
		"bad router":    {"GATEWAY_COMMANDS_ENABLED": "true", "GATEWAY_ROUTER_ID": "x"},
	}
	for name, values := range cases {
		if _, err := loadCommandSettings(env(values)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestStartCommandRunnerRequiresFullConfiguration(t *testing.T) {
	err := startCommandRunner(nil, nil, env(map[string]string{"GATEWAY_COMMANDS_ENABLED": "true"}))
	if err == nil || !strings.Contains(err.Error(), "requires CLOUD_API_URL") {
		t.Fatalf("got %v", err)
	}
}
