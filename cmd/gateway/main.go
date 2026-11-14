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

	info, err := os.Stat(tokenPath)
	if err != nil {
		return fmt.Errorf("read Gateway token file metadata: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("Gateway token path must be a regular file")
	}
	if info.Mode().Perm()&0077 != 0 {
		return errors.New("Gateway token file permissions are too broad; use chmod 600")
	}
	tokenBytes, err := os.ReadFile(tokenPath)
	if err != nil {
		return fmt.Errorf("read Gateway token file: %w", err)
	}
	token := strings.TrimSpace(string(tokenBytes))
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
