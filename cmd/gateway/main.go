package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/chadracklonde/mtandao-gateway/internal/cloud"
)

type healthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

func main() {
	logger := log.New(os.Stdout, "mtandao-gateway ", log.LstdFlags|log.LUTC)
	if len(os.Args) > 1 {
		if os.Args[1] == "enroll" && len(os.Args) == 2 {
			if err := enrollGateway(context.Background(), logger); err != nil {
				logger.Fatal(err)
			}
			return
		}
		logger.Fatal("usage: gateway [enroll]")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := startCloudHeartbeat(ctx, logger); err != nil {
		logger.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(healthResponse{Status: "ok", Service: "mtandao-gateway"})
	})

	addr := os.Getenv("GATEWAY_LISTEN_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8090"
	}
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	logger.Printf("health endpoint listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatal(err)
	}
}

func enrollGateway(ctx context.Context, logger *log.Logger) error {
	baseURL := strings.TrimSpace(os.Getenv("CLOUD_API_URL"))
	enrollmentPath := strings.TrimSpace(os.Getenv("GATEWAY_ENROLLMENT_TOKEN_FILE"))
	tokenPath := strings.TrimSpace(os.Getenv("GATEWAY_TOKEN_FILE"))
	if baseURL == "" || enrollmentPath == "" || tokenPath == "" {
		return errors.New("CLOUD_API_URL, GATEWAY_ENROLLMENT_TOKEN_FILE and GATEWAY_TOKEN_FILE are required for enrollment")
	}
	enrollmentAbs, err := filepath.Abs(enrollmentPath)
	if err != nil {
		return fmt.Errorf("resolve enrollment token path: %w", err)
	}
	tokenAbs, err := filepath.Abs(tokenPath)
	if err != nil {
		return fmt.Errorf("resolve Gateway token path: %w", err)
	}
	if enrollmentAbs == tokenAbs {
		return errors.New("enrollment token file and permanent Gateway token file must be different")
	}
	if _, err := os.Lstat(tokenPath); err == nil {
		return errors.New("Gateway token file already exists; refusing to consume another enrollment token")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect Gateway token destination: %w", err)
	}

	enrollmentToken, err := cloud.ReadSecretFile(enrollmentPath)
	if err != nil {
		return fmt.Errorf("read enrollment token: %w", err)
	}
	result, err := cloud.RedeemEnrollment(ctx, baseURL, enrollmentToken, nil)
	if err != nil {
		return fmt.Errorf("Cloud enrollment failed: %w", err)
	}
	if err := cloud.WriteSecretFile(tokenPath, result.GatewayToken); err != nil {
		return fmt.Errorf("Cloud enrollment succeeded but permanent token could not be stored securely: %w", err)
	}
	if err := os.Remove(enrollmentPath); err != nil {
		logger.Printf("WARNING: enrollment token file could not be removed; delete it securely: %s", enrollmentPath)
	}
	logger.Printf("Gateway enrolled successfully: router=%s site=%s", result.RouterID, result.SiteID)
	logger.Printf("Permanent credential stored in %s; keep this file private", tokenAbs)
	return nil
}

func startCloudHeartbeat(ctx context.Context, logger *log.Logger) error {
	baseURL := strings.TrimSpace(os.Getenv("CLOUD_API_URL"))
	tokenPath := strings.TrimSpace(os.Getenv("GATEWAY_TOKEN_FILE"))
	if baseURL == "" && tokenPath == "" {
		logger.Printf("Cloud heartbeat disabled: CLOUD_API_URL and GATEWAY_TOKEN_FILE are not configured")
		return nil
	}
	if baseURL == "" || tokenPath == "" {
		return errors.New("CLOUD_API_URL and GATEWAY_TOKEN_FILE must be configured together")
	}

	token, err := cloud.ReadSecretFile(tokenPath)
	if err != nil {
		return fmt.Errorf("read Gateway token: %w", err)
	}
	client, err := cloud.NewClient(baseURL, token, nil)
	if err != nil {
		return fmt.Errorf("configure Cloud client: %w", err)
	}

	interval := 60 * time.Second
	if raw := strings.TrimSpace(os.Getenv("GATEWAY_HEARTBEAT_INTERVAL")); raw != "" {
		seconds, parseErr := strconv.Atoi(raw)
		if parseErr != nil || seconds < 15 || seconds > 3600 {
			return errors.New("GATEWAY_HEARTBEAT_INTERVAL must be between 15 and 3600 seconds")
		}
		interval = time.Duration(seconds) * time.Second
	}
	go cloud.RunHeartbeat(ctx, client, interval, logger)
	return nil
}
