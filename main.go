package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"policy-workbench/internal/application"
	"policy-workbench/internal/demo"
	"policy-workbench/internal/httpapi"
	"policy-workbench/internal/storage/jsonfile"
)

//go:embed web/* TASK.md
var embeddedFiles embed.FS

func main() {
	addr := flag.String("addr", "127.0.0.1:8094", "HTTP listen address")
	data := flag.String("data", ".data/state-v4.json", "JSON state file; use a distinct file for each process")
	flag.Parse()
	catalog := demo.Catalog()
	seedWorkspace := func(now time.Time) application.WorkspaceState { return demo.Workspace(catalog, now) }
	repository, err := jsonfile.Open(*data, seedWorkspace(time.Now()))
	if err != nil {
		log.Fatal(err)
	}
	service, err := application.NewService(repository, catalog, seedWorkspace)
	if err != nil {
		log.Fatal(err)
	}
	assets, err := fs.Sub(embeddedFiles, "web")
	if err != nil {
		log.Fatal(err)
	}
	brief, err := embeddedFiles.ReadFile("TASK.md")
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{Addr: *addr, Handler: httpapi.NewHandler(service, assets, brief), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	fmt.Printf("\n  Policy Workbench\n  http://%s\n  State: %s\n\n", *addr, *data)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
