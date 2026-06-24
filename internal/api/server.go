package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/reelwarden/reelwarden/internal/compliance"
	"github.com/reelwarden/reelwarden/internal/config"
)

type Server struct {
	cfg       config.Config
	db        *sql.DB
	mux       *http.ServeMux
	startedAt time.Time
}

func NewServer(cfg config.Config, db *sql.DB) *Server {
	s := &Server{cfg: cfg, db: db, mux: http.NewServeMux(), startedAt: time.Now().UTC()}
	s.routes()
	return s
}
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /health", s.health)
	s.mux.HandleFunc("GET /api/compliance/gates", s.gates)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "reelwarden", "started_at": s.startedAt.Format(time.RFC3339)})
}
func (s *Server) gates(w http.ResponseWriter, r *http.Request) {
	result := compliance.EvaluateTMDBAI(compliance.RuntimeInputs{TMDBEnabled: s.cfg.Metadata.Providers.TMDB.Enabled, AIEnabled: s.cfg.AI.Enabled, TMDBAIStatus: s.cfg.Compliance.TMDBAI})
	writeJSON(w, http.StatusOK, []compliance.GateResult{result})
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
