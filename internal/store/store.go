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
	ParsedIdentityID     string `json:"parsed_identity_id,omitempty"`
	MatchState           string `json:"match_state"`
	ConfirmedCandidateID string `json:"confirmed_candidate_id,omitempty"`
	CreatedAt            string `json:"created_at"`
	UpdatedAt            string `json:"updated_at"`
}

// ScoreBand is the §14.7 confidence band used to route the resolver ladder.
type ScoreBand = string

const (
	BandHigh   ScoreBand = "high"   // rank_score >= 0.95; preselect, still human-confirm
	BandMedium ScoreBand = "medium" // 0.80 <= rank_score < 0.95; require review
	BandLow    ScoreBand = "low"    // rank_score < 0.80; escalate or unresolved
)

// §14.7 band thresholds. rank_score is an ordering heuristic, not a probability.
const (
	BandHighThreshold   = 0.95 // rank_score >= 0.95 -> high
	BandMediumThreshold = 0.80 // 0.80 <= rank_score < 0.95 -> medium
)

// BandFor maps a rank_score to its §14.7 ScoreBand.
func BandFor(rankScore float64) ScoreBand {
	switch {
	case rankScore >= BandHighThreshold:
		return BandHigh
	case rankScore >= BandMediumThreshold:
		return BandMedium
	default:
		return BandLow
	}
}

// ParsedIdentity is the file-name parse result (§9.2, extended per the resolver
// pipeline "Data model contract"). It supersedes parser.Result. Confidence is a
// heuristic score, NOT a probability. NormalizedTitle is for comparison only and
// must never be shown to the user (§12.3).
type ParsedIdentity struct {
	ID              string            `json:"id"`
	MediaAssetID    string            `json:"media_asset_id"`
	RawTitle        string            `json:"raw_title"`         // title region of the file name, pre-normalization
	NormalizedTitle string            `json:"normalized_title"`  // §12.3 normalization; comparison use only, never display
	ComparisonKeys  []string          `json:"comparison_keys"`   // extra match keys, e.g. simplified/traditional fold (§12.3)
	Year            int               `json:"year,omitempty"`    // 0 if absent; title-aware (§12.4)
	Edition         string            `json:"edition,omitempty"` // Director's Cut / Extended / Remux-as-edition (§9.2)
	ReleaseGroup    string            `json:"release_group,omitempty"`
	TechnicalTags   []string          `json:"technical_tags"`            // §12.2 full set
	MediaTypeHint   string            `json:"media_type_hint,omitempty"` // "" | movie | tv | tv_liveaction | ova | special
	ParentDirName   string            `json:"parent_dir_name,omitempty"` // local untrusted; type/title signal (R2)
	Hypotheses      []QueryHypothesis `json:"hypotheses"`                // ordered most-constrained first (§12.6)
	Confidence      float64           `json:"confidence"`                // heuristic, NOT a probability (§9.2)
	ParserVersion   string            `json:"parser_version"`
	State           string            `json:"state"` // parsed | unresolved
	CreatedAt       string            `json:"created_at"`
}

// ToIdentitySeed builds a minimal ParsedIdentity from an already-scanned asset,
// pre-populating the fields the resolver ladder starts from (R0 input preservation).
// The richer fields (NormalizedTitle, ComparisonKeys, Hypotheses, ...) are filled
// by the parser/normalization rungs.
func (a MediaAsset) ToIdentitySeed() ParsedIdentity {
	return ParsedIdentity{
		MediaAssetID: a.ID,
		RawTitle:     a.ParsedTitle,
		Year:         a.ParsedYear,
		State:        "parsed",
	}
}

// QueryHypothesis is a single constrained provider-query hypothesis emitted by
// the parser/AI. Deterministic code issues the query; AI never does (§14.1, §7.1).
type QueryHypothesis struct {
	Title       string            `json:"title"`
	Year        int               `json:"year,omitempty"`
	MediaType   string            `json:"media_type,omitempty"`   // constrains the provider query (movie vs tv endpoint)
	ExternalIDs map[string]string `json:"external_ids,omitempty"` // local-only exact IDs, e.g. imdb/tmdb/tvdb
	Source      string            `json:"source"`                 // rule | parent_dir | romanized | comparison_key | local_external_id | ai_repair
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
	mu              sync.RWMutex
	roots           map[string]LibraryRoot
	assets          map[string]MediaAsset
	assetsByPath    map[string]string
	candidates      map[string][]Candidate
	plans           map[string]ActionPlan
	identities      map[string]ParsedIdentity
	identityByAsset map[string]string
}

func New() *Store {
	return &Store{roots: map[string]LibraryRoot{}, assets: map[string]MediaAsset{}, assetsByPath: map[string]string{}, candidates: map[string][]Candidate{}, plans: map[string]ActionPlan{}, identities: map[string]ParsedIdentity{}, identityByAsset: map[string]string{}}
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

// UpsertParsedIdentity stores a ParsedIdentity keyed by MediaAssetID (one per
// asset). It links the owning MediaAsset via ParsedIdentityID when present.
func (s *Store) UpsertParsedIdentity(p ParsedIdentity) ParsedIdentity {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.identityByAsset[p.MediaAssetID]; ok && p.MediaAssetID != "" {
		old := s.identities[id]
		p.ID = id
		p.CreatedAt = old.CreatedAt
	} else {
		if p.ID == "" {
			p.ID = NewID("pid")
		}
		p.CreatedAt = now()
		if p.MediaAssetID != "" {
			s.identityByAsset[p.MediaAssetID] = p.ID
		}
	}
	if p.State == "" {
		p.State = "parsed"
	}
	s.identities[p.ID] = p
	if a, ok := s.assets[p.MediaAssetID]; ok && a.ParsedIdentityID != p.ID {
		a.ParsedIdentityID = p.ID
		a.UpdatedAt = now()
		s.assets[a.ID] = a
	}
	return p
}

// ParsedIdentity returns the identity by its id.
func (s *Store) ParsedIdentity(id string) (ParsedIdentity, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.identities[id]
	return p, ok
}

// ParsedIdentityForAsset returns the identity associated with a MediaAsset.
func (s *Store) ParsedIdentityForAsset(assetID string) (ParsedIdentity, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.identityByAsset[assetID]
	if !ok {
		return ParsedIdentity{}, false
	}
	p, ok := s.identities[id]
	return p, ok
}
