package main

import "testing"

func TestLoadConfigDefaultsLocalBindAddr(t *testing.T) {
	t.Setenv("TS_PROXY_MAPPINGS", "8999=service.tailnet.ts.net:8999")
	t.Setenv("TS_PROXY_LOCAL_ADDR", "")
	clearAuthEnv(t)

	config, err := loadConfigFromEnv()
	if err != nil {
		t.Fatalf("loadConfigFromEnv returned error: %v", err)
	}

	if config.LocalBindAddr != "127.0.0.1" {
		t.Fatalf("expected default local bind address 127.0.0.1, got %q", config.LocalBindAddr)
	}

	if !config.AcceptRoutes {
		t.Fatal("expected accept routes to default to true")
	}
}

func TestLoadConfigReadsLocalBindAddr(t *testing.T) {
	t.Setenv("TS_PROXY_MAPPINGS", "8999=service.tailnet.ts.net:8999")
	t.Setenv("TS_PROXY_LOCAL_ADDR", "0.0.0.0")
	clearAuthEnv(t)

	config, err := loadConfigFromEnv()
	if err != nil {
		t.Fatalf("loadConfigFromEnv returned error: %v", err)
	}

	if config.LocalBindAddr != "0.0.0.0" {
		t.Fatalf("expected configured local bind address 0.0.0.0, got %q", config.LocalBindAddr)
	}
}

func TestLoadConfigCanDisableAcceptRoutes(t *testing.T) {
	t.Setenv("TS_PROXY_MAPPINGS", "8999=service.tailnet.ts.net:8999")
	t.Setenv("TS_PROXY_ACCEPT_ROUTES", "false")
	clearAuthEnv(t)

	config, err := loadConfigFromEnv()
	if err != nil {
		t.Fatalf("loadConfigFromEnv returned error: %v", err)
	}

	if config.AcceptRoutes {
		t.Fatal("expected accept routes to be disabled")
	}
}

func TestLoadConfigReadsAdvertiseTags(t *testing.T) {
	t.Setenv("TS_PROXY_MAPPINGS", "8999=service.tailnet.ts.net:8999")
	t.Setenv("TS_PROXY_ADVERTISE_TAGS", "tag:ci,tag:proxy")
	clearAuthEnv(t)

	config, err := loadConfigFromEnv()
	if err != nil {
		t.Fatalf("loadConfigFromEnv returned error: %v", err)
	}

	if len(config.AdvertiseTags) != 2 {
		t.Fatalf("expected 2 advertise tags, got %d", len(config.AdvertiseTags))
	}

	if config.AdvertiseTags[0] != "tag:ci" || config.AdvertiseTags[1] != "tag:proxy" {
		t.Fatalf("unexpected advertise tags: %#v", config.AdvertiseTags)
	}
}

func TestBuildAuthKeyUsesOAuthURLParams(t *testing.T) {
	t.Setenv("TS_PROXY_AUTH_EPHEMERAL", "false")
	t.Setenv("TS_PROXY_AUTH_PREAUTHORIZED", "true")
	t.Setenv("TS_PROXY_AUTH_BASE_URL", "https://api.tailscale.example")

	authKey, err := buildAuthKey("tskey-client-secret", "")
	if err != nil {
		t.Fatalf("buildAuthKey returned error: %v", err)
	}

	want := "tskey-client-secret?baseURL=https%3A%2F%2Fapi.tailscale.example&ephemeral=false&preauthorized=true"
	if authKey != want {
		t.Fatalf("expected auth key %q, got %q", want, authKey)
	}
}

func TestBuildAuthKeyPreservesExistingQuery(t *testing.T) {
	t.Setenv("TS_PROXY_AUTH_PREAUTHORIZED", "true")

	authKey, err := buildAuthKey("tskey-client-secret?ephemeral=false", "")
	if err != nil {
		t.Fatalf("buildAuthKey returned error: %v", err)
	}

	want := "tskey-client-secret?ephemeral=false&preauthorized=true"
	if authKey != want {
		t.Fatalf("expected auth key %q, got %q", want, authKey)
	}
}

func TestParseMappings(t *testing.T) {
	mappings, err := parseMappings("5432=postgres.tailnet.ts.net:5432,8080=100.64.0.10:80")
	if err != nil {
		t.Fatalf("parseMappings returned error: %v", err)
	}

	if len(mappings) != 2 {
		t.Fatalf("expected 2 mappings, got %d", len(mappings))
	}

	if mappings[0].ListenPort != 5432 || mappings[0].TargetAddr != "postgres.tailnet.ts.net:5432" {
		t.Fatalf("unexpected first mapping: %#v", mappings[0])
	}

	if mappings[1].ListenPort != 8080 || mappings[1].TargetAddr != "100.64.0.10:80" {
		t.Fatalf("unexpected second mapping: %#v", mappings[1])
	}
}

func TestParseMappingsRejectsDuplicateListenPorts(t *testing.T) {
	_, err := parseMappings("5432=postgres-a.tailnet.ts.net:5432;5432=postgres-b.tailnet.ts.net:5432")
	if err == nil {
		t.Fatal("expected duplicate listen port error")
	}
}

func TestParseMappingsRejectsMissingTargetPort(t *testing.T) {
	_, err := parseMappings("8080=100.64.0.10")
	if err == nil {
		t.Fatal("expected missing target port error")
	}
}

func TestParseAdvertiseTagsRejectsMissingPrefix(t *testing.T) {
	_, err := parseAdvertiseTags("ci")
	if err == nil {
		t.Fatal("expected missing tag prefix error")
	}
}

func clearAuthEnv(t *testing.T) {
	t.Helper()

	t.Setenv("TS_AUTHKEY", "")
	t.Setenv("TS_PROXY_AUTH_KEY", "")
	t.Setenv("TS_PROXY_AUTH_EPHEMERAL", "")
	t.Setenv("TS_PROXY_AUTH_PREAUTHORIZED", "")
	t.Setenv("TS_PROXY_AUTH_BASE_URL", "")
}
