package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"tailscale.com/ipn"
	"tailscale.com/tsnet"
)

const (
	defaultHostname      = "ts-proxy"
	defaultStateDir      = "./tsnet-state"
	defaultLocalBindAddr = "127.0.0.1"
)

type Config struct {
	Hostname      string
	StateDir      string
	AuthKey       string
	AdvertiseTags []string
	AcceptRoutes  bool
	LocalBindAddr string
	Mappings      []Mapping
}

type Mapping struct {
	ListenPort int
	TargetAddr string
}

type Server struct {
	config    Config
	tsnet     *tsnet.Server
	listeners []net.Listener
	wg        sync.WaitGroup
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("ts-proxy")
	log.Printf("Version: %s", Version)
	log.Printf("Build Time: %s", BuildTime)
	log.Printf("Git Commit: %s", GitCommit)
	log.Printf("Module: %s", ModuleName)
	log.Println()

	config, err := loadConfigFromEnv()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	server := &Server{config: config}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := server.Start(ctx); err != nil {
		log.Fatalf("failed to start proxy: %v", err)
	}

	<-ctx.Done()
	log.Println("shutdown requested")
	server.Shutdown()
}

func loadConfigFromEnv() (Config, error) {
	hostname := strings.TrimSpace(os.Getenv("TS_PROXY_HOSTNAME"))
	if hostname == "" {
		hostname = defaultHostname
	}

	stateDir := strings.TrimSpace(os.Getenv("TS_PROXY_STATE_DIR"))
	if stateDir == "" {
		stateDir = defaultStateDir
	}

	localBindAddr := strings.TrimSpace(os.Getenv("TS_PROXY_LOCAL_ADDR"))
	if localBindAddr == "" {
		localBindAddr = defaultLocalBindAddr
	}

	mappings, err := parseMappings(os.Getenv("TS_PROXY_MAPPINGS"))
	if err != nil {
		return Config{}, err
	}

	authKey, err := buildAuthKey(os.Getenv("TS_PROXY_AUTH_KEY"), os.Getenv("TS_AUTHKEY"))
	if err != nil {
		return Config{}, err
	}

	advertiseTags, err := parseAdvertiseTags(os.Getenv("TS_PROXY_ADVERTISE_TAGS"))
	if err != nil {
		return Config{}, err
	}

	acceptRoutes, err := parseBoolEnvDefault("TS_PROXY_ACCEPT_ROUTES", true)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Hostname:      hostname,
		StateDir:      stateDir,
		AuthKey:       authKey,
		AdvertiseTags: advertiseTags,
		AcceptRoutes:  acceptRoutes,
		LocalBindAddr: localBindAddr,
		Mappings:      mappings,
	}, nil
}

func parseMappings(raw string) ([]Mapping, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("TS_PROXY_MAPPINGS is required, e.g. 5432=postgres.tailnet.ts.net:5432,8080=100.64.0.10:80")
	}

	entries := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n'
	})

	mappings := make([]Mapping, 0, len(entries))
	seenPorts := make(map[int]struct{}, len(entries))

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		left, right, ok := strings.Cut(entry, "=")
		if !ok {
			return nil, fmt.Errorf("invalid mapping %q: expected listenPort=targetHost:targetPort", entry)
		}

		listenPort, err := parsePort(strings.TrimSpace(left))
		if err != nil {
			return nil, fmt.Errorf("invalid listen port in mapping %q: %w", entry, err)
		}

		targetAddr := strings.TrimSpace(right)
		if err := validateTargetAddr(targetAddr); err != nil {
			return nil, fmt.Errorf("invalid target in mapping %q: %w", entry, err)
		}

		if _, exists := seenPorts[listenPort]; exists {
			return nil, fmt.Errorf("duplicate listen port %d", listenPort)
		}
		seenPorts[listenPort] = struct{}{}

		mappings = append(mappings, Mapping{
			ListenPort: listenPort,
			TargetAddr: targetAddr,
		})
	}

	if len(mappings) == 0 {
		return nil, errors.New("TS_PROXY_MAPPINGS did not contain any mappings")
	}

	return mappings, nil
}

func buildAuthKey(primary string, fallback string) (string, error) {
	authKey := strings.TrimSpace(primary)
	if authKey == "" {
		authKey = strings.TrimSpace(fallback)
	}
	if authKey == "" {
		return "", nil
	}

	params := url.Values{}
	for envName, paramName := range map[string]string{
		"TS_PROXY_AUTH_EPHEMERAL":     "ephemeral",
		"TS_PROXY_AUTH_PREAUTHORIZED": "preauthorized",
		"TS_PROXY_AUTH_BASE_URL":      "baseURL",
	} {
		value := strings.TrimSpace(os.Getenv(envName))
		if value == "" {
			continue
		}

		if paramName == "ephemeral" || paramName == "preauthorized" {
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return "", fmt.Errorf("%s must be a boolean: %w", envName, err)
			}
			value = strconv.FormatBool(parsed)
		}

		params.Set(paramName, value)
	}

	if len(params) == 0 {
		return authKey, nil
	}

	separator := "?"
	if strings.Contains(authKey, "?") {
		separator = "&"
	}
	return authKey + separator + params.Encode(), nil
}

func parseAdvertiseTags(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	entries := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n'
	})

	tags := make([]string, 0, len(entries))
	seenTags := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		tag := strings.TrimSpace(entry)
		if tag == "" {
			continue
		}
		if !strings.HasPrefix(tag, "tag:") {
			return nil, fmt.Errorf("advertise tag %q must start with tag:", tag)
		}
		if _, exists := seenTags[tag]; exists {
			return nil, fmt.Errorf("duplicate advertise tag %q", tag)
		}
		seenTags[tag] = struct{}{}
		tags = append(tags, tag)
	}

	return tags, nil
}

func parseBoolEnvDefault(name string, defaultValue bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue, nil
	}

	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", name, err)
	}

	return value, nil
}

func parsePort(raw string) (int, error) {
	port, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("port must be between 1 and 65535, got %d", port)
	}
	return port, nil
}

func validateTargetAddr(addr string) error {
	host, portRaw, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("expected host:port: %w", err)
	}
	if strings.TrimSpace(host) == "" {
		return errors.New("target host cannot be empty")
	}
	_, err = parsePort(portRaw)
	return err
}

func (s *Server) Start(ctx context.Context) error {
	s.tsnet = &tsnet.Server{
		Hostname:      s.config.Hostname,
		Dir:           s.config.StateDir,
		AuthKey:       s.config.AuthKey,
		AdvertiseTags: s.config.AdvertiseTags,
	}

	status, err := s.tsnet.Up(ctx)
	if err != nil {
		return fmt.Errorf("failed to bring up Tailscale: %w", err)
	}

	if s.config.AcceptRoutes {
		if err := s.acceptRoutes(ctx); err != nil {
			return err
		}
	}

	log.Printf("tsnet node ready: hostname=%s tailscaleIPs=%v", s.config.Hostname, status.TailscaleIPs)
	if len(s.config.AdvertiseTags) > 0 {
		log.Printf("advertising tags: %s", strings.Join(s.config.AdvertiseTags, ","))
	}
	log.Printf("accept routes: %t", s.config.AcceptRoutes)

	for _, mapping := range s.config.Mappings {
		if err := s.startMapping(ctx, mapping); err != nil {
			s.Shutdown()
			return err
		}
	}

	return nil
}

func (s *Server) acceptRoutes(ctx context.Context) error {
	localClient, err := s.tsnet.LocalClient()
	if err != nil {
		return fmt.Errorf("failed to create Tailscale local client: %w", err)
	}

	_, err = localClient.EditPrefs(ctx, &ipn.MaskedPrefs{
		Prefs: ipn.Prefs{
			RouteAll: true,
		},
		RouteAllSet: true,
	})
	if err != nil {
		return fmt.Errorf("failed to accept Tailscale routes: %w", err)
	}

	return nil
}

func (s *Server) startMapping(ctx context.Context, mapping Mapping) error {
	tailnetAddr := fmt.Sprintf(":%d", mapping.ListenPort)
	tailnetListener, err := s.tsnet.Listen("tcp", tailnetAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on Tailscale port %d: %w", mapping.ListenPort, err)
	}

	s.listeners = append(s.listeners, tailnetListener)
	log.Printf("forwarding tailnet port %d -> %s", mapping.ListenPort, mapping.TargetAddr)
	s.startAcceptLoop(ctx, tailnetListener, mapping)

	localAddr := net.JoinHostPort(s.config.LocalBindAddr, strconv.Itoa(mapping.ListenPort))
	localListener, err := net.Listen("tcp", localAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on local address %s: %w", localAddr, err)
	}

	s.listeners = append(s.listeners, localListener)
	log.Printf("forwarding local address %s -> %s", localListener.Addr(), mapping.TargetAddr)
	s.startAcceptLoop(ctx, localListener, mapping)

	return nil
}

func (s *Server) startAcceptLoop(ctx context.Context, listener net.Listener, mapping Mapping) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.acceptLoop(ctx, listener, mapping)
	}()
}

func (s *Server) acceptLoop(ctx context.Context, listener net.Listener, mapping Mapping) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			log.Printf("failed to accept connection on port %d: %v", mapping.ListenPort, err)
			continue
		}

		go s.handleConnection(ctx, conn, mapping)
	}
}

func (s *Server) handleConnection(ctx context.Context, clientConn net.Conn, mapping Mapping) {
	defer clientConn.Close()

	targetConn, err := s.tsnet.Dial(ctx, "tcp", mapping.TargetAddr)
	if err != nil {
		log.Printf("failed to connect %s to target %s: %v", clientConn.RemoteAddr(), mapping.TargetAddr, err)
		return
	}
	defer targetConn.Close()

	log.Printf("forwarding %s -> %s", clientConn.RemoteAddr(), mapping.TargetAddr)

	done := make(chan struct{}, 2)
	copyConn := func(dst net.Conn, src net.Conn) {
		_, _ = io.Copy(dst, src)
		_ = dst.Close()
		_ = src.Close()
		done <- struct{}{}
	}

	go copyConn(targetConn, clientConn)
	go copyConn(clientConn, targetConn)

	<-done
	log.Printf("closed %s -> %s", clientConn.RemoteAddr(), mapping.TargetAddr)
}

func (s *Server) Shutdown() {
	for _, listener := range s.listeners {
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Printf("failed to close listener %s: %v", listener.Addr(), err)
		}
	}

	if s.tsnet != nil {
		if err := s.tsnet.Close(); err != nil {
			log.Printf("failed to close tsnet server: %v", err)
		}
	}

	s.wg.Wait()
	log.Println("shutdown complete")
}
