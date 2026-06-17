package main

import (
	"crypto/tls"
	"flag"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"non24.app/server/internal/api"
	"non24.app/server/internal/config"
	"non24.app/server/internal/store"
)

func main() {
	configPath := flag.String("config", "", "path to JSON config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	st, err := store.Open(filepath.Join(cfg.DataDir, "zeitboardd.db"), cfg.DataKey)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	tlsConfig, err := cfg.TLSConfig()
	if err != nil {
		log.Fatalf("configure TLS: %v", err)
	}
	handler := api.New(st, cfg.EnrollmentSecret).Handler()
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
		log.Fatalf("listen: %v", err)
	}
	log.Printf("zeitboardd listening on %s", cfg.ListenAddress)
	if cfg.UsesSelfSignedDevCert {
		log.Printf("using runtime-generated localhost TLS certificate")
	}
	if err := server.Serve(tls.NewListener(listener, tlsConfig)); err != nil && err != http.ErrServerClosed {
		log.Fatalf("serve: %v", err)
	}
}
