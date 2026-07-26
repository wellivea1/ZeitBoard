package daemon

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"non24.app/server/internal/api"
	"non24.app/server/internal/config"
	"non24.app/server/internal/provider"
	"non24.app/server/internal/store"
)

const shutdownTimeout = 15 * time.Second

func ValidateConfig(configPath string) error {
	_, _, _, _, err := loadRuntimeConfig(configPath)
	return err
}

func loadRuntimeConfig(configPath string) (config.Config, provider.LLM, provider.Status, *tls.Config, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return config.Config{}, nil, provider.Status{}, nil, fmt.Errorf("load config: %w", err)
	}
	llm, providerStatus, err := provider.New(provider.Config{
		Name:     provider.Name(cfg.Assistant.Provider),
		Model:    cfg.Assistant.Model,
		APIKey:   cfg.Assistant.APIKey,
		Endpoint: cfg.Assistant.Endpoint,
	})
	if err != nil {
		return config.Config{}, nil, provider.Status{}, nil, fmt.Errorf("configure provider: %w", err)
	}
	tlsConfig, err := cfg.TLSConfig()
	if err != nil {
		return config.Config{}, nil, provider.Status{}, nil, fmt.Errorf("configure TLS: %w", err)
	}
	return cfg, llm, providerStatus, tlsConfig, nil
}

func Serve(configPath string, stop <-chan struct{}) error {
	return serve(configPath, stop, nil)
}

func serve(configPath string, stop <-chan struct{}, ready chan<- struct{}) error {
	cfg, llm, providerStatus, tlsConfig, err := loadRuntimeConfig(configPath)
	if err != nil {
		return err
	}
	st, err := store.Open(filepath.Join(cfg.DataDir, "zeitboardd.db"), cfg.DataKey)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()
	handler := api.New(st, cfg.EnrollmentSecret, api.WithProvider(llm, providerStatus)).Handler()
	server := &http.Server{
		Addr:              cfg.ListenAddress,
		Handler:           handler,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	listener, err := net.Listen("tcp", cfg.ListenAddress)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	log.Printf("zeitboardd listening on %s", cfg.ListenAddress)
	if cfg.UsesSelfSignedDevCert {
		log.Printf("using runtime-generated localhost TLS certificate")
	}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(tls.NewListener(listener, tlsConfig))
	}()
	if ready != nil {
		close(ready)
	}

	if stop == nil {
		return normalizeServeError(<-serveErr)
	}
	select {
	case err := <-serveErr:
		return normalizeServeError(err)
	case <-stop:
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		shutdownErr := server.Shutdown(ctx)
		if shutdownErr != nil {
			// Shutdown does not force-close connections when its context expires.
			// Close guarantees the Serve goroutine terminates before we return.
			_ = server.Close()
			<-serveErr
			return fmt.Errorf("shutdown: %w", shutdownErr)
		}
		serveErrValue := <-serveErr
		return normalizeServeError(serveErrValue)
	}
}

func normalizeServeError(err error) error {
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
