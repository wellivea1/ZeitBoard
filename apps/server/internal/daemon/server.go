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

	"non24.app/core/recompute"
	"non24.app/server/internal/analysis"
	"non24.app/server/internal/api"
	"non24.app/server/internal/config"
	"non24.app/server/internal/portal"
	"non24.app/server/internal/portalbridge"
	"non24.app/server/internal/provider"
	"non24.app/server/internal/readmodel"
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

	options := []api.Option{api.WithProvider(llm, providerStatus)}
	var publicHandler http.Handler
	if cfg.Portal.Enabled {
		portalStore, portalErr := portal.Open(filepath.Join(cfg.DataDir, "zeitboard-portal.db"), cfg.DataKey)
		if portalErr != nil {
			return fmt.Errorf("open portal store: %w", portalErr)
		}
		defer portalStore.Close()

		materializer := portalbridge.Materializer{
			Sleep:    readmodel.SleepReader{Store: st},
			Profiles: portalStore,
			Sink:     portalStore,
		}
		requestBridge := &portalbridge.RequestBridge{Portal: portalStore, Private: st}
		portalHandler, handlerErr := portal.NewHandler(portal.HandlerConfig{
			Store:        portalStore,
			PublicOrigin: cfg.Portal.PublicOrigin,
			NotifyRequestCreated: func() {
				if err := requestBridge.Pump(context.Background()); err != nil {
					log.Printf("portal: request bridge pump failed: %v", err)
				}
			},
		})
		if handlerErr != nil {
			return fmt.Errorf("configure portal: %w", handlerErr)
		}
		// The analysis loop (ADR-0033). It reconciles at startup — so a link
		// opened before the first sync of the day shows today's freshness — and
		// then recomputes both when evidence changes and when the freshness
		// verdict is due to change on its own, which is the case no amount of
		// request-driven materialization ever reached.
		//
		// A dedicated stop channel rather than the caller's: `stop` may be nil,
		// and these defers must run before the store closes (LIFO) so no worker
		// touches a closed database.
		recomputeWorker := &analysis.Worker{
			Orchestrator: recompute.Orchestrator{
				Analysis: analysis.Portal{Materializer: materializer},
				Journal:  store.RecomputeJournal{Store: st},
			},
		}
		analysisStop := make(chan struct{})
		analysisDone := recomputeWorker.Start(analysisStop)
		defer func() {
			close(analysisStop)
			<-analysisDone
		}()

		maintenanceStop := make(chan struct{})
		maintenanceDone := startPortalMaintenance(portalStore, requestBridge, maintenanceStop)
		defer func() {
			close(maintenanceStop)
			<-maintenanceDone
		}()

		options = append(options, api.WithPortal(api.PortalConfig{
			Store:        portalStore,
			PublicOrigin: cfg.Portal.PublicOrigin,
			Materializer: materializer,
			Requests:     requestBridge,
			Recompute:    recomputeWorker,
		}))
		publicHandler = portalHandler.Routes()
		log.Printf("zeitboardd portal enabled at %s/p/", cfg.Portal.PublicOrigin)
	}

	handler := mountPortal(api.New(st, cfg.EnrollmentSecret, options...).Handler(), publicHandler)
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

const (
	portalMaintenanceInterval = time.Hour

	// portalBridgeInterval is the recovery path, not the happy path: a
	// request is pumped as soon as it is created and a decision as soon as it
	// is made. This timer exists so a request that arrived while the bridge
	// was failing still reaches the owner without anyone intervening.
	portalBridgeInterval = time.Minute
)

// startPortalMaintenance expires sessions and rate buckets, enforces the
// access-audit retention window, and drains the request outbox in both
// directions. Retention that nothing enforces is not retention, and an outbox
// nothing drains is a queue that silently stops. The returned channel closes
// once the worker has stopped.
func startPortalMaintenance(store *portal.Store, bridge *portalbridge.RequestBridge, stop <-chan struct{}) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		sweep := time.NewTicker(portalMaintenanceInterval)
		defer sweep.Stop()
		pump := time.NewTicker(portalBridgeInterval)
		defer pump.Stop()

		drain := func() {
			if bridge == nil {
				return
			}
			if err := bridge.Pump(context.Background()); err != nil {
				// A failure here leaves requests in their honest `queued`
				// state; the next tick retries.
				log.Printf("portal: request bridge pump failed: %v", err)
			}
		}
		purge := func() {
			if err := store.PurgeExpired(context.Background(), time.Now().UTC()); err != nil {
				log.Printf("portal: maintenance sweep failed: %v", err)
			}
		}
		purge()
		drain()
		for {
			select {
			case <-sweep.C:
				purge()
			case <-pump.C:
				drain()
			case <-stop:
				return
			}
		}
	}()
	return done
}

// mountPortal routes /p/ to the public handler and everything else to the
// device API. When the portal is disabled publicHandler is nil and the private
// handler is returned unchanged, so no /p/ path exists at all.
func mountPortal(private http.Handler, publicHandler http.Handler) http.Handler {
	if publicHandler == nil {
		return private
	}
	mux := http.NewServeMux()
	mux.Handle("/", private)
	mux.Handle("/p/", publicHandler)
	return mux
}

func normalizeServeError(err error) error {
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
