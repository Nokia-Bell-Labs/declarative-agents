package main

import (
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	ui "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/cmd/analyzer/ui"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the analyzer web server",
	RunE: func(cmd *cobra.Command, args []string) error {
		addr, _ := cmd.Flags().GetString("addr")
		dataDir, _ := cmd.Flags().GetString("data")
		configsDir, _ := cmd.Flags().GetString("configs")

		if dataDir != "" {
			if abs, err := filepath.Abs(dataDir); err == nil {
				dataDir = abs
			}
		}
		if configsDir != "" {
			if abs, err := filepath.Abs(configsDir); err == nil {
				configsDir = abs
			}
		}

		srv := &server{
			dataDir:    dataDir,
			configsDir: configsDir,
		}

		mux := http.NewServeMux()

		mux.HandleFunc("GET /api/v1/health", srv.handleHealth)
		mux.HandleFunc("GET /api/v1/sessions", srv.handleListSessions)
		mux.HandleFunc("GET /api/v1/sessions/{suite}/{ts}", srv.handleGetSession)
		mux.HandleFunc("GET /api/v1/sessions/{suite}/{ts}/points", srv.handleListPoints)
		mux.HandleFunc("GET /api/v1/sessions/{suite}/{ts}/points/{pointId}", srv.handleGetTrace)

		mux.Handle("/", spaHandler(ui.Assets()))

		log.Printf("analyzer listening on %s (data=%s, configs=%s)", addr, dataDir, configsDir)
		return http.ListenAndServe(addr, mux)
	},
}

func init() {
	serveCmd.Flags().String("addr", ":8080", "listen address")
	serveCmd.Flags().String("data", "", "path to eval-results directory")
	serveCmd.Flags().String("configs", "", "path to configs directory")
}

type server struct {
	dataDir    string
	configsDir string
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func spaHandler(assets fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(assets))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		f, err := assets.Open(path)
		if err != nil {
			serveIndexHTML(w, assets)
			return
		}
		f.Close()

		fileServer.ServeHTTP(w, r)
	})
}

func serveIndexHTML(w http.ResponseWriter, assets fs.FS) {
	data, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		http.Error(w, "index.html not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

type apiResponse struct {
	Data  interface{} `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
}

func writeData(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusOK, apiResponse{Data: data})
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiResponse{Error: msg})
}
