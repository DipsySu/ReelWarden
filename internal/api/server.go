package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/reelwarden/reelwarden/internal/compliance"
	"github.com/reelwarden/reelwarden/internal/config"
	"github.com/reelwarden/reelwarden/internal/metadata"
	"github.com/reelwarden/reelwarden/internal/planner"
	"github.com/reelwarden/reelwarden/internal/scanner"
	"github.com/reelwarden/reelwarden/internal/store"
)

type Server struct {
	cfg       config.Config
	st        *store.Store
	mux       *http.ServeMux
	startedAt time.Time
}

func NewServer(cfg config.Config, st *store.Store) *Server {
	s := &Server{cfg: cfg, st: st, mux: http.NewServeMux(), startedAt: time.Now().UTC()}
	s.routes()
	return s
}
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) method(method string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			writeError(w, http.StatusMethodNotAllowed, "API_METHOD_NOT_ALLOWED", errors.New("method not allowed"))
			return
		}
		next(w, r)
	}
}

func (s *Server) routes() {
	s.mux.HandleFunc("/health", s.method(http.MethodGet, s.health))
	s.mux.HandleFunc("/api/config/runtime", s.method(http.MethodGet, s.runtimeConfig))
	s.mux.HandleFunc("/api/compliance/gates", s.method(http.MethodGet, s.gates))
	s.mux.HandleFunc("/api/library_roots", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.listRoots(w, r)
		case http.MethodPost:
			s.addRoot(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "API_METHOD_NOT_ALLOWED", errors.New("method not allowed"))
		}
	})
	s.mux.HandleFunc("/api/scans", s.method(http.MethodPost, s.scan))
	s.mux.HandleFunc("/api/assets", s.method(http.MethodGet, s.assets))
	s.mux.HandleFunc("/api/assets/", s.assetSubroutes)
	s.mux.HandleFunc("/api/plans", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			s.plans(w, r)
		case http.MethodPost:
			s.createPlan(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "API_METHOD_NOT_ALLOWED", errors.New("method not allowed"))
		}
	})
}
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "reelwarden", "started_at": s.startedAt.Format(time.RFC3339)})
}
func (s *Server) runtimeConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.cfg.RuntimeView())
}
func (s *Server) gates(w http.ResponseWriter, r *http.Request) {
	result := compliance.EvaluateTMDBAI(compliance.RuntimeInputs{TMDBEnabled: s.cfg.Metadata.Providers.TMDB.Enabled, AIEnabled: s.cfg.AI.Enabled, TMDBAIStatus: s.cfg.Compliance.TMDBAI})
	writeJSON(w, http.StatusOK, []compliance.GateResult{result})
}
func (s *Server) listRoots(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.st.Roots())
}
func (s *Server) addRoot(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "CFG_REQUEST_INVALID", err)
		return
	}
	root, err := s.st.AddRoot(req.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, "FS_LIBRARY_ROOT_INVALID", err)
		return
	}
	writeJSON(w, http.StatusCreated, root)
}
func (s *Server) scan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		LibraryRootID string `json:"library_root_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "SCAN_REQUEST_INVALID", err)
		return
	}
	root, ok := s.st.Root(req.LibraryRootID)
	if !ok {
		writeError(w, http.StatusNotFound, "SCAN_LIBRARY_ROOT_NOT_FOUND", errors.New("library root not found"))
		return
	}
	res, err := scanner.Scan(context.Background(), s.st, root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SCAN_FAILED", err)
		return
	}
	for _, a := range res.Assets {
		s.st.SaveCandidates(a.ID, metadata.MockCandidates(a))
	}
	writeJSON(w, http.StatusOK, res)
}
func (s *Server) assets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.st.Assets())
}
func (s *Server) assetSubroutes(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/assets/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 2 && parts[1] == "candidates" && r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, s.st.Candidates(parts[0]))
		return
	}
	if len(parts) == 2 && parts[1] == "confirm" && r.Method == http.MethodPost {
		var req struct {
			CandidateID string `json:"candidate_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "MATCH_REQUEST_INVALID", err)
			return
		}
		a, err := s.st.Confirm(parts[0], req.CandidateID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "MATCH_CONFIRM_FAILED", err)
			return
		}
		writeJSON(w, http.StatusOK, a)
		return
	}
	writeError(w, http.StatusNotFound, "API_NOT_FOUND", errors.New("route not found"))
}
func (s *Server) createPlan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AssetID string `json:"asset_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "PLAN_REQUEST_INVALID", err)
		return
	}
	p, err := planner.CreateDryRun(s.st, req.AssetID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "PLAN_CREATE_FAILED", err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}
func (s *Server) plans(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.st.Plans())
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, code string, err error) {
	writeJSON(w, status, map[string]any{"error_code": code, "message": err.Error()})
}
