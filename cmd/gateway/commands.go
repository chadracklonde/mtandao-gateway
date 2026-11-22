package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/chadracklonde/mtandao-gateway/internal/cloud"
	"github.com/chadracklonde/mtandao-gateway/internal/commands"
	"github.com/chadracklonde/mtandao-gateway/internal/executor"
	"github.com/chadracklonde/mtandao-gateway/internal/routeros"
)

const defaultStateDir = "/var/lib/mtandao-gateway"

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type commandSettings struct {
	Enabled      bool
	StateDir     string
	PollInterval time.Duration
	SiteID       string
	RouterID     string
}

// loadCommandSettings reads the command-execution settings. Execution is off
// unless GATEWAY_COMMANDS_ENABLED=true, because it lets the Cloud change router state.
func loadCommandSettings(getenv func(string) string) (commandSettings, error) {
	settings := commandSettings{StateDir: defaultStateDir, PollInterval: 10 * time.Second}
	switch strings.ToLower(strings.TrimSpace(getenv("GATEWAY_COMMANDS_ENABLED"))) {
	case "", "false", "0", "no":
		return settings, nil
	case "true", "1", "yes":
		settings.Enabled = true
	default:
		return settings, errors.New("GATEWAY_COMMANDS_ENABLED must be true or false")
	}
	if dir := strings.TrimSpace(getenv("GATEWAY_STATE_DIR")); dir != "" {
		if !filepath.IsAbs(dir) {
			return settings, errors.New("GATEWAY_STATE_DIR must be an absolute path")
		}
		settings.StateDir = filepath.Clean(dir)
	}
	if raw := strings.TrimSpace(getenv("GATEWAY_COMMAND_POLL_INTERVAL")); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds < 2 || seconds > 300 {
			return settings, errors.New("GATEWAY_COMMAND_POLL_INTERVAL must be between 2 and 300 seconds")
		}
		settings.PollInterval = time.Duration(seconds) * time.Second
	}
	for name, target := range map[string]*string{"GATEWAY_SITE_ID": &settings.SiteID, "GATEWAY_ROUTER_ID": &settings.RouterID} {
		value := strings.TrimSpace(getenv(name))
		if value != "" && !uuidPattern.MatchString(value) {
			return settings, fmt.Errorf("%s must be a UUID", name)
		}
		*target = strings.ToLower(value)
	}
	return settings, nil
}

// startCommandRunner starts Cloud command execution when it is enabled.
func startCommandRunner(ctx context.Context, logger *log.Logger, getenv func(string) string) error {
	settings, err := loadCommandSettings(getenv)
	if err != nil {
		return err
	}
	if !settings.Enabled {
		logger.Printf("Cloud command execution disabled (set GATEWAY_COMMANDS_ENABLED=true to enable)")
		return nil
	}

	baseURL := strings.TrimSpace(getenv("CLOUD_API_URL"))
	tokenPath := strings.TrimSpace(getenv("GATEWAY_TOKEN_FILE"))
	routerConfigPath := strings.TrimSpace(getenv("ROUTEROS_CONFIG_FILE"))
	if baseURL == "" || tokenPath == "" || routerConfigPath == "" {
		return errors.New("command execution requires CLOUD_API_URL, GATEWAY_TOKEN_FILE and ROUTEROS_CONFIG_FILE")
	}
	token, err := cloud.ReadSecretFile(tokenPath)
	if err != nil {
		return fmt.Errorf("read Gateway token: %w", err)
	}
	client, err := cloud.NewClient(baseURL, token, nil)
	if err != nil {
		return fmt.Errorf("configure Cloud client: %w", err)
	}
	routerConfig, err := routeros.ReadConfig(routerConfigPath)
	if err != nil {
		return fmt.Errorf("load RouterOS configuration: %w", err)
	}
	ledger, err := executor.OpenLedger(filepath.Join(settings.StateDir, "commands.json"))
	if err != nil {
		return fmt.Errorf("open command ledger: %w", err)
	}
	connect := func(ctx context.Context) (executor.Router, func(), error) {
		session, err := routeros.OpenSession(ctx, routerConfig)
		if err != nil {
			return nil, nil, err
		}
		return session, func() { _ = session.Close() }, nil
	}
	exec, err := executor.New(ledger, connect, executor.Target{SiteID: settings.SiteID, RouterID: settings.RouterID})
	if err != nil {
		return err
	}
	go commands.NewRunner(client, exec, ledger, settings.PollInterval, logger).Run(ctx)
	logger.Printf("Cloud command execution enabled (poll every %s)", settings.PollInterval)
	return nil
}
