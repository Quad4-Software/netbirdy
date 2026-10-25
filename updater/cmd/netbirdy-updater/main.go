package main

import (
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/netbirdio/netbird/updater"
)

func main() {
	cfg, err := updater.Load()
	if err != nil {
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
