package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthcheck(t *testing.T) {
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthy.Close()
	if err := healthcheck(healthy.URL); err != nil {
		t.Fatalf("healthy endpoint failed: %v", err)
	}

	unhealthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer unhealthy.Close()
	if err := healthcheck(unhealthy.URL); err == nil {
		t.Fatal("non-OK endpoint passed healthcheck")
	}
}

func TestOptionalBoolEnv(t *testing.T) {
	t.Setenv("SETUP_ENABLED", "")
	if value, err := optionalBoolEnv("SETUP_ENABLED", true); err != nil || !value {
		t.Fatalf("default: value=%v err=%v", value, err)
	}
	t.Setenv("SETUP_ENABLED", "false")
	if value, err := optionalBoolEnv("SETUP_ENABLED", true); err != nil || value {
		t.Fatalf("false: value=%v err=%v", value, err)
	}
	t.Setenv("SETUP_ENABLED", "yes")
	if _, err := optionalBoolEnv("SETUP_ENABLED", true); err == nil {
		t.Fatal("invalid boolean was accepted")
	}
}

func TestValidateTokensRequiresIndependentLongValues(t *testing.T) {
	apiToken := strings.Repeat("a", 32)
	setupToken := strings.Repeat("b", 32)
	if err := validateTokens(apiToken, setupToken); err != nil {
		t.Fatalf("valid independent tokens rejected: %v", err)
	}
	if err := validateTokens(strings.Repeat("a", 31), setupToken); err == nil {
		t.Fatal("short API token accepted")
	}
	if err := validateTokens(apiToken, apiToken); err == nil {
		t.Fatal("identical API and setup tokens accepted")
	}
}
