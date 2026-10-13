package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

// Run starts Local Dream from the current project or release directory.
func Run() error {
	root, _ := os.Getwd()
	data := filepath.Join(root, "data")
	if err := os.MkdirAll(filepath.Join(data, "assets"), 0755); err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	s := &Server{
		ctx:        ctx,
		root:       root,
		comfyURL:   env("COMFYUI_URL", "http://127.0.0.1:8188"),
		comfyInput: env("COMFYUI_INPUT", `D:\Comfy-Desktop\ComfyUI-Shared\input`),
		httpClient: &http.Client{Timeout: 30 * time.Second},
		assets:     []Asset{},
		jobs:       []Generation{},
	}
	// Import while the old in-memory server is still running; never stop it first.
	if len(os.Args) == 3 && os.Args[1] == "--import-legacy" {
		if err := s.openStore(); err != nil {
			return err
		}
		defer s.db.Close()
		return s.importLegacy(os.Args[2])
	}
	addr := env("LOCALDREAM_ADDR", "127.0.0.1:8790")
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("Local Dream 端口不可用（不会启动另一份 ComfyUI）: %w", err)
	}
	defer listener.Close()
	if err := s.openStore(); err != nil {
		return err
	}
	defer s.db.Close()
	cleanup, err := s.ensureComfy(ctx)
	if err != nil {
		return err
	}
	defer cleanup()
	mux := http.NewServeMux()
	mux.Handle("/mcp", s.mcpHandler())
	mux.HandleFunc("/api/assets", s.assetsHandler)
	mux.HandleFunc("/api/assets/", s.deleteAssetHandler)
	mux.HandleFunc("/api/projects", s.projectsHandler)
	mux.HandleFunc("/api/projects/", s.projectHandler)
	mux.HandleFunc("/api/generations", s.generationsHandler)
	mux.HandleFunc("/api/generations/", s.generationHandler)
	mux.HandleFunc("/api/providers/models", s.modelsHandler)
	mux.HandleFunc("/api/agent/interpret", s.agentHandler)
	mux.HandleFunc("/api/agent/turn", s.agentTurnHandler)
	mux.HandleFunc("/api/config/provider", s.providerConfigHandler)
	mux.HandleFunc("/api/config/long-video", s.longVideoSettingsHandler)
	mux.HandleFunc("/api/long-videos", s.longVideosHandler)
	mux.HandleFunc("/api/long-videos/", s.longVideoHandler)
	mux.HandleFunc("/api/workflows/import", s.importWorkflowHandler)
	mux.Handle("/media/", http.StripPrefix("/media/", http.FileServer(http.Dir(filepath.Join(data, "assets")))))
	mux.Handle("/", http.FileServer(http.Dir(filepath.Join(root, "frontend", "dist"))))
	s.recoverJobs()
	s.startLongVideoDispatcher()
	log.Printf("Local Dream listening at http://%s", addr)
	server := &http.Server{Handler: cors(mux), ReadHeaderTimeout: 10 * time.Second}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case <-ctx.Done():
	case err = <-done:
		cancel()
	}
	shutdown, stop := context.WithTimeout(context.Background(), 15*time.Second)
	defer stop()
	if shutdownErr := server.Shutdown(shutdown); shutdownErr != nil {
		_ = server.Close()
	}
	s.workers.Wait()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
