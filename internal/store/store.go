package store

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type LibraryRoot struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	Mode      string `json:"mode"`
	CreatedAt string `json:"created_at"`
}
type MediaAsset struct {
	ID                   string `json:"id"`
	LibraryRootID        string `json:"library_root_id"`
	RelativePath         string `json:"relative_path"`
	SizeBytes            int64  `json:"size_bytes"`
	ModifiedAt           string `json:"modified_at"`
	ScanState            string `json:"scan_state"`
	ParsedTitle          string `json:"parsed_title"`
	ParsedYear           int    `json:"parsed_year,omitempty"`
	MatchState           string `json:"match_state"`
	ConfirmedCandidateID string `json:"confirmed_candidate_id,omitempty"`
	CreatedAt            string `json:"created_at"`
	UpdatedAt            string `json:"updated_at"`
}
type Candidate struct {
	ID         string     `json:"id"`
	AssetID    string     `json:"asset_id"`
	Provider   string     `json:"provider"`
	ProviderID string     `json:"provider_id"`
	Title      string     `json:"title"`
	Year       int        `json:"year,omitempty"`
	Score      int        `json:"score"`
	ScoreBand  string     `json:"score_band"`
	Evidence   []Evidence `json:"evidence"`
}
type Evidence struct {
	Group   string `json:"group"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Points  int    `json:"points"`
}
type ActionPlan struct {
	ID                 string   `json:"id"`
	AssetID            string   `json:"asset_id"`
	SourceRelativePath string   `json:"source_relative_path"`
	TargetRelativePath string   `json:"target_relative_path"`
	DryRun             bool     `json:"dry_run"`
	State              string   `json:"state"`
	CreatedAt          string   `json:"created_at"`
	Warnings           []string `json:"warnings"`
}

type Store struct {
	mu           sync.RWMutex
	roots        map[string]LibraryRoot
	assets       map[string]MediaAsset
	assetsByPath map[string]string
	candidates   map[string][]Candidate
	plans        map[string]ActionPlan
}

func New() *Store {
	return &Store{roots: map[string]LibraryRoot{}, assets: map[string]MediaAsset{}, assetsByPath: map[string]string{}, candidates: map[string][]Candidate{}, plans: map[string]ActionPlan{}}
}
func NewID(prefix string) string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return prefix + "_" + hex.EncodeToString(b[:])
}
func now() string { return time.Now().UTC().Format(time.RFC3339) }

func (s *Store) AddRoot(path string) (LibraryRoot, error) {
	if path == "" {
		return LibraryRoot{}, errors.New("FS_LIBRARY_ROOT_REQUIRED")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return LibraryRoot{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.roots {
		if r.Path == abs {
			return r, nil
		}
	}
	r := LibraryRoot{ID: NewID("root"), Path: abs, Mode: "read_only", CreatedAt: now()}
	s.roots[r.ID] = r
	return r, nil
}
func (s *Store) Roots() []LibraryRoot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]LibraryRoot, 0, len(s.roots))
	for _, r := range s.roots {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out
}
func (s *Store) Root(id string) (LibraryRoot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.roots[id]
	return r, ok
}
func (s *Store) UpsertAsset(a MediaAsset) MediaAsset {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := a.LibraryRootID + "/" + a.RelativePath
	if id, ok := s.assetsByPath[key]; ok {
		old := s.assets[id]
		a.ID = id
		a.CreatedAt = old.CreatedAt
	} else {
		a.ID = NewID("asset")
		a.CreatedAt = now()
		s.assetsByPath[key] = a.ID
	}
	a.UpdatedAt = now()
	if a.MatchState == "" {
		a.MatchState = "needs_review"
	}
	s.assets[a.ID] = a
	return a
}
func (s *Store) Assets() []MediaAsset {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]MediaAsset, 0, len(s.assets))
	for _, a := range s.assets {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RelativePath < out[j].RelativePath })
	return out
}
func (s *Store) Asset(id string) (MediaAsset, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.assets[id]
	return a, ok
}
func (s *Store) SaveCandidates(assetID string, c []Candidate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.candidates[assetID] = c
}
func (s *Store) Candidates(assetID string) []Candidate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Candidate{}, s.candidates[assetID]...)
}
func (s *Store) Confirm(assetID, candidateID string) (MediaAsset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.assets[assetID]
	if !ok {
		return MediaAsset{}, errors.New("MATCH_ASSET_NOT_FOUND")
	}
	found := false
	for _, c := range s.candidates[assetID] {
		if c.ID == candidateID {
			found = true
			break
		}
	}
	if !found {
		return MediaAsset{}, errors.New("MATCH_CANDIDATE_NOT_FOUND")
	}
	a.MatchState = "confirmed"
	a.ConfirmedCandidateID = candidateID
	a.UpdatedAt = now()
	s.assets[assetID] = a
	return a, nil
}
func (s *Store) SavePlan(p ActionPlan) ActionPlan {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.ID == "" {
		p.ID = NewID("plan")
	}
	p.CreatedAt = now()
	s.plans[p.ID] = p
	return p
}
func (s *Store) Plans() []ActionPlan {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ActionPlan, 0, len(s.plans))
	for _, p := range s.plans {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out
}
