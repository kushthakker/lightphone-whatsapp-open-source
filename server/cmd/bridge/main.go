package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"light-whatsapp/internal/bridge"
)

func main() {
	if len(os.Args) == 3 && os.Args[1] == "-healthcheck" {
		if err := healthcheck(os.Args[2]); err != nil {
			log.Fatal("bridge healthcheck failed")
		}
		return
	}
	if err := run(); err != nil {
		log.Fatal("bridge failed to start or stopped unexpectedly")
	}
}

func run() error {
	listen := env("LISTEN_ADDR", "127.0.0.1:8080")
	dbPath := env("DATABASE_PATH", "./data/bridge.db")
	apiToken, err := requiredEnv("API_TOKEN")
	if err != nil {
		return err
	}
	setupToken, err := requiredEnv("SETUP_TOKEN")
	if err != nil {
		return err
	}
	if err := validateTokens(apiToken, setupToken); err != nil {
		return err
	}
	setupEnabled, err := optionalBoolEnv("SETUP_ENABLED", true)
	if err != nil {
		return err
	}
	publicBaseURLRaw, err := requiredEnv("PUBLIC_BASE_URL")
	if err != nil {
		return err
	}
	publicBaseURL, err := bridge.ValidatePublicBaseURL(publicBaseURLRaw, os.Getenv("DEBUG") == "true")
	if err != nil {
		return err
	}
	groupPolicy, err := bridge.GroupPolicyFromJSON(os.Getenv("GROUP_MODE"), os.Getenv("GROUP_ALLOWLIST"))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		return err
	}
	mediaDir := dbPath + ".media"
	if err := os.MkdirAll(mediaDir, 0700); err != nil {
		return err
	}
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_busy_timeout=5000&_journal_mode=WAL", dbPath)
	store, err := bridge.OpenStore(dsn, groupPolicy)
	if err != nil {
		return err
	}
	defer store.Close()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	wa, err := bridge.NewWhatsApp(ctx, dsn, store, mediaDir, groupPolicy)
	if err != nil {
		return fmt.Errorf("initialize WhatsApp: %w", err)
	}
	defer wa.Close()
	if err := wa.Start(ctx); err != nil {
		return fmt.Errorf("start WhatsApp: %w", err)
	}
	server := &http.Server{Addr: listen, Handler: bridge.NewAPI(store, wa, wa, wa, mediaDir, publicBaseURL, apiToken, setupToken, setupEnabled), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 100 * time.Second, IdleTimeout: 60 * time.Second}
	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("bridge listening on %s", listen)
		serverErrors <- server.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func validateTokens(apiToken, setupToken string) error {
	if len(apiToken) < 32 || len(setupToken) < 32 {
		return errors.New("API_TOKEN and SETUP_TOKEN must each be at least 32 characters")
	}
	if apiToken == setupToken {
		return errors.New("API_TOKEN and SETUP_TOKEN must be different")
	}
	return nil
}

func healthcheck(rawURL string) error {
	request, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return errors.New("invalid healthcheck URL")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return errors.New("healthcheck request failed")
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	if response.StatusCode != http.StatusOK {
		return errors.New("healthcheck endpoint did not return OK")
	}
	return nil
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func requiredEnv(name string) (string, error) {
	value := os.Getenv(name)
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func optionalBoolEnv(name string, fallback bool) (bool, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	if value == "true" {
		return true, nil
	}
	if value == "false" {
		return false, nil
	}
	return false, fmt.Errorf("%s must be true or false", name)
}
