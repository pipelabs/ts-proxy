package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tailscale.com/ipn"
)

func TestLoadConfigReadsHealthAddr(t *testing.T) {
	t.Setenv("TS_PROXY_MAPPINGS", "8999=service.tailnet.ts.net:8999")
	t.Setenv("TS_PROXY_HEALTH_ADDR", " 0.0.0.0:9000 ")
	clearAuthEnv(t)

	config, err := loadConfigFromEnv()
	if err != nil {
		t.Fatalf("loadConfigFromEnv returned error: %v", err)
	}

	if config.HealthAddr != "0.0.0.0:9000" {
		t.Fatalf("expected health address 0.0.0.0:9000, got %q", config.HealthAddr)
	}
}

func TestLoadConfigHealthAddrDefaultsToDisabled(t *testing.T) {
	t.Setenv("TS_PROXY_MAPPINGS", "8999=service.tailnet.ts.net:8999")
	t.Setenv("TS_PROXY_HEALTH_ADDR", "")
	clearAuthEnv(t)

	config, err := loadConfigFromEnv()
	if err != nil {
		t.Fatalf("loadConfigFromEnv returned error: %v", err)
	}

	if config.HealthAddr != "" {
		t.Fatalf("expected health address to be empty, got %q", config.HealthAddr)
	}
}

func TestIsFatalState(t *testing.T) {
	fatal := []ipn.State{ipn.NeedsLogin, ipn.NeedsMachineAuth, ipn.Stopped}
	for _, state := range fatal {
		if !isFatalState(state) {
			t.Fatalf("expected %s to be fatal", state)
		}
	}

	recoverable := []ipn.State{ipn.NoState, ipn.InUseOtherUser, ipn.Starting, ipn.Running}
	for _, state := range recoverable {
		if isFatalState(state) {
			t.Fatalf("expected %s not to be fatal", state)
		}
	}
}

func TestHealthzReportsRunningAsOK(t *testing.T) {
	server := &Server{}
	server.backend.set(ipn.Running, nil)

	recorder := httptest.NewRecorder()
	server.handleHealthz(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
}

func TestHealthzReportsNonRunningStateAsUnavailable(t *testing.T) {
	server := &Server{}
	server.backend.set(ipn.NeedsLogin, nil)

	recorder := httptest.NewRecorder()
	server.handleHealthz(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "NeedsLogin") {
		t.Fatalf("expected body to name the state, got %q", recorder.Body.String())
	}
}

func TestHealthzReportsWatchErrorAsUnavailable(t *testing.T) {
	server := &Server{}
	server.backend.set(ipn.NoState, errors.New("bus closed"))

	recorder := httptest.NewRecorder()
	server.handleHealthz(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "bus closed") {
		t.Fatalf("expected body to include the error, got %q", recorder.Body.String())
	}
}

func TestHealthzBeforeFirstNotificationIsUnavailable(t *testing.T) {
	server := &Server{}

	recorder := httptest.NewRecorder()
	server.handleHealthz(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", recorder.Code)
	}
}

func TestFailKeepsOnlyFirstError(t *testing.T) {
	server := &Server{fatal: make(chan error, 1)}
	server.fail(errors.New("first"))
	server.fail(errors.New("second"))

	err := <-server.Fatal()
	if err.Error() != "first" {
		t.Fatalf("expected first error, got %q", err)
	}

	select {
	case extra := <-server.Fatal():
		t.Fatalf("expected no further errors, got %q", extra)
	default:
	}
}
