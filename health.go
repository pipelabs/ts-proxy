package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"tailscale.com/ipn"
)

// Backend state the proxy has last observed on the IPN bus. tsnet only reports
// state once at Up; after that the node can be logged out from the control
// plane (device deleted, key expired) with no local signal, so every dial just
// times out. Watching the bus turns that silent failure into a process exit.
type backendState struct {
	mu    sync.RWMutex
	state ipn.State
	err   error
}

func (b *backendState) set(state ipn.State, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.state = state
	b.err = err
}

func (b *backendState) get() (ipn.State, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.state, b.err
}

// isFatalState reports whether the node can no longer recover on its own.
// Starting and NoState occur transiently on reconnects and are not fatal.
func isFatalState(state ipn.State) bool {
	switch state {
	case ipn.NeedsLogin, ipn.NeedsMachineAuth, ipn.Stopped:
		return true
	default:
		return false
	}
}

func (s *Server) watchBackendState(ctx context.Context) error {
	localClient, err := s.tsnet.LocalClient()
	if err != nil {
		return fmt.Errorf("failed to create Tailscale local client: %w", err)
	}

	watcher, err := localClient.WatchIPNBus(ctx, ipn.NotifyInitialState|ipn.NotifyNoPrivateKeys)
	if err != nil {
		return fmt.Errorf("failed to watch Tailscale backend state: %w", err)
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer watcher.Close()

		for {
			notify, err := watcher.Next()
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				s.backend.set(ipn.NoState, err)
				s.fail(fmt.Errorf("Tailscale backend state watch ended: %w", err))
				return
			}

			if notify.State == nil {
				continue
			}

			state := *notify.State
			s.backend.set(state, nil)
			log.Printf("tailscale backend state: %s", state)

			if isFatalState(state) {
				s.fail(fmt.Errorf("tailscale node is %s and cannot recover without re-authenticating", state))
				return
			}
		}
	}()

	return nil
}

// fail records the first fatal error; later ones are dropped.
func (s *Server) fail(err error) {
	select {
	case s.fatal <- err:
	default:
	}
}

func (s *Server) Fatal() <-chan error {
	return s.fatal
}

func (s *Server) startHealthServer(ctx context.Context) error {
	if s.config.HealthAddr == "" {
		return nil
	}

	listener, err := net.Listen("tcp", s.config.HealthAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on health address %s: %w", s.config.HealthAddr, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	s.health = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("health endpoint on http://%s/healthz", listener.Addr())

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.health.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) && ctx.Err() == nil {
			s.fail(fmt.Errorf("health server stopped: %w", err))
		}
	}()

	return nil
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	state, err := s.backend.get()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	switch {
	case err != nil:
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "unhealthy: %v\n", err)
	case state != ipn.Running:
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "unhealthy: tailscale backend state %s\n", state)
	default:
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	}
}
