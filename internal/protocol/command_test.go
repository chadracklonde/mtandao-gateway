package protocol

import (
	"testing"
	"time"
)

func validCommand(now time.Time) Command {
	return Command{
		CommandID: "cmd-1", SiteID: "site-1", GatewayID: "gateway-1",
		RouterID: "router-1", Operation: "health.check",
		IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute),
		Payload: map[string]any{},
	}
}

func TestValidateAcceptsAllowedUnexpiredCommand(t *testing.T) {
	now := time.Now()
	if err := validCommand(now).Validate(now); err != nil {
		t.Fatalf("expected valid command, got %v", err)
	}
}

func TestValidateRejectsExpiredCommand(t *testing.T) {
	now := time.Now()
	command := validCommand(now)
	command.ExpiresAt = now.Add(-time.Second)
	if err := command.Validate(now); err == nil {
		t.Fatal("expected expired command to be rejected")
	}
}

func TestValidateRejectsUnknownOperation(t *testing.T) {
	now := time.Now()
	command := validCommand(now)
	command.Operation = "system.shell"
	if err := command.Validate(now); err == nil {
		t.Fatal("expected unknown operation to be rejected")
	}
}

func TestValidateRejectsMissingSite(t *testing.T) {
	now := time.Now()
	command := validCommand(now)
	command.SiteID = ""
	if err := command.Validate(now); err == nil {
		t.Fatal("expected missing site to be rejected")
	}
}
