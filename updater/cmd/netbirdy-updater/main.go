package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/netbirdio/netbird/updater"
	"github.com/netbirdio/netbird/util/sandbox"
)

func httpClient() *http.Client { return &http.Client{Timeout: 5 * time.Second} }

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		healthCheck()
	}
	cfg, err := updater.Load()
	if err != nil {
		log.Fatal(err)
	}
	if _, err := sandbox.Apply(sandbox.Policy{
		Name:          "updater",
		ReadWriteDirs: sandbox.NonEmpty(cfg.DataDir, cfg.BackupDir),
	}); err != nil {
		log.Fatal(err)
	}
	run, err := updater.NewRunner(cfg)
	if err != nil {
		log.Fatal(err)
	}
	srv := updater.NewServer(cfg, run)
	log.Printf("listening on %s", cfg.Listen)
	if err := srv.Listen().ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Print(err)
		os.Exit(1)
	}
}

func healthCheck() {
	resp, err := httpClient().Get("http://127.0.0.1:8089/healthz")
	if err != nil || resp.StatusCode != 200 {
		os.Exit(1)
	}
	resp.Body.Close()
	os.Exit(0)
}
