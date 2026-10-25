package main

import (
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/netbirdio/netbird/upload-server/server"
	"github.com/netbirdio/netbird/util"
	"github.com/netbirdio/netbird/util/sandbox"
)

func main() {
	err := util.InitLog("info", util.LogConsole)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	storeDir := "/var/lib/netbird"
	if d := os.Getenv("STORE_DIR"); d != "" {
		storeDir = d
	}
	if _, err = sandbox.Apply(sandbox.Policy{Name: "upload server", ReadWriteDirs: []string{storeDir}}); err != nil {
		log.Fatalf("Failed to apply landlock sandbox: %v", err)
	}

	srv := server.NewServer()
	if err = srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("Failed to start server: %v", err)
	}
}
