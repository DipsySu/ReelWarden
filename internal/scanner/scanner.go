// Package scanner enumerates video files under a library root and, for each
// file, runs the deterministic resolver ladder (R0..R5, authority §14.9) to
// populate a store.ParsedIdentity and ranked candidate matches.
//
// Resilience (§0.2/§11.5): the scan itself never fails because of a provider,
// ffprobe, or AI problem. Each per-file resolve is isolated -- a degraded rung
// records evidence and the file simply ends up unresolved/metadata_pending,
// while the scan as a whole keeps going and reports every asset it indexed.
package scanner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/reelwarden/reelwarden/internal/matcher"
	"github.com/reelwarden/reelwarden/internal/metadata"
	"github.com/reelwarden/reelwarden/internal/resolver"
	"github.com/reelwarden/reelwarden/internal/store"
)

var videoExt = map[string]bool{".mkv": true, ".mp4": true, ".avi": true, ".mov": true, ".m4v": true, ".wmv": true, ".ts": true, ".m2ts": true, ".webm": true}

type Result struct {
	Scanned int                `json:"scanned"`
	Assets  []store.MediaAsset `json:"assets"`
}

// Options tunes a scan. Both fields are optional; the zero value yields the
// deterministic mock-provider default with AI disabled (Local Only, §7.7).
type Options struct {
	// Query is the deterministic provider door injected into the resolver. When
	// nil, the built-in mock provider door is used so the read-only MVP works
	// offline (§13.2). AI never receives this -- it is deterministic-only.
	Query resolver.ProviderQuery
	// LLM optionally enables the R4 local AI filename-repair rung. nil keeps AI
	// off (the default, §0.2: core works without AI). It receives LOCAL signals
	// only and emits hypotheses only.
	LLM resolver.LLMClient
}

// resolver.LLMClient alias keeps callers from importing airepair directly; the
// resolver re-exports the airepair.LLMClient interface shape.

// Scan walks root, indexes every video file, and resolves each one. It is the
// Integrate entry point used by the API. Provider/ffprobe/AI failures during a
// per-file resolve are isolated and never abort the walk.
func Scan(ctx context.Context, st *store.Store, root store.LibraryRoot) (Result, error) {
	return ScanWithOptions(ctx, st, root, Options{})
}

// ScanWithOptions is Scan with an injected provider door / optional local AI.
func ScanWithOptions(ctx context.Context, st *store.Store, root store.LibraryRoot, opts Options) (Result, error) {
	query := opts.Query
	if query == nil {
		query = mockProviderQuery // offline deterministic default (§13.2).
	}

	var res Result
	err := filepath.WalkDir(root.Path, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // unreadable entry: skip, never abort the scan (§0.2).
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if d.IsDir() {
			return nil
		}
		if !videoExt[strings.ToLower(filepath.Ext(p))] {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(root.Path, p)
		if err != nil {
			return nil
		}
		relSlash := filepath.ToSlash(rel)

		// Index the asset first so a later resolve failure never loses the file.
		a := st.UpsertAsset(store.MediaAsset{
			LibraryRootID: root.ID,
			RelativePath:  relSlash,
			SizeBytes:     info.Size(),
			ModifiedAt:    info.ModTime().UTC().Format(time.RFC3339),
			ScanState:     "scanned",
			MatchState:    "needs_review",
		})

		// Run the deterministic ladder. ResolveAsset already isolates rung
		// failures internally; this never returns an error.
		out := resolver.ResolveAsset(resolver.Input{
			Asset:         a,
			AbsPath:       p,
			RelPath:       relSlash,
			ParentDirName: parentDirName(p, root.Path),
			Query:         query,
			LLM:           opts.LLM,
			Ctx:           ctx,
		})

		// Persist the identity and candidates. Update the asset's denormalized
		// parse fields + match routing state from the resolver result.
		id := out.Identity
		id.MediaAssetID = a.ID
		stored := st.UpsertParsedIdentity(id)

		a.ParsedTitle = id.RawTitle
		a.ParsedYear = id.Year
		a.ParsedIdentityID = stored.ID
		a.MatchState = matchStateFor(out)
		a = st.UpsertAsset(a)

		st.SaveCandidates(a.ID, out.Candidates)

		res.Scanned++
		res.Assets = append(res.Assets, a)
		return nil
	})
	return res, err
}

// matchStateFor maps the resolver outcome to the asset's match_state. v0.1.1
// NEVER auto-confirms (§14.7): the best a resolved file gets is "needs_review"
// (a human-confirm preselection). Unresolved files are surfaced for review too;
// when no provider candidate exists at all the file is metadata_pending (§11.5).
func matchStateFor(out resolver.Result) string {
	if out.State == "resolved" {
		return "needs_review" // preselected best, still human-confirm (§14.7).
	}
	if len(out.Candidates) == 0 {
		return "metadata_pending" // no provider candidates (§11.5).
	}
	return "needs_review"
}

// parentDirName returns the immediate parent directory name of a file, treating
// the library root itself as "no parent" (empty). Local untrusted signal (§7.2).
func parentDirName(absFile, root string) string {
	dir := filepath.Dir(absFile)
	if dir == "" || dir == root {
		return ""
	}
	return filepath.Base(dir)
}

// mockProviderQuery adapts the deterministic mock provider (§13.2) to the
// resolver's ProviderQuery door. It builds a throwaway asset carrying the
// hypothesis title/year, asks the mock for candidates, and projects them into
// matcher.ProviderCandidate. This is deterministic, offline, and provider-side
// only -- the AI never calls it.
//
// TODO(integrate): when the real metadata gateway / TMDB adapter lands (§13.1
// MovieCandidate / MovieMetadata), inject its deterministic search here as the
// resolver.ProviderQuery instead of the mock.
func mockProviderQuery(_ context.Context, h store.QueryHypothesis) ([]matcher.ProviderCandidate, error) {
	if strings.TrimSpace(h.Title) == "" {
		return nil, nil
	}
	seed := store.MediaAsset{ParsedTitle: h.Title, ParsedYear: h.Year}
	out := make([]matcher.ProviderCandidate, 0, 2)
	for _, c := range metadata.MockCandidates(seed) {
		out = append(out, matcher.ProviderCandidate{
			Provider:       c.Provider,
			ProviderItemID: c.ProviderID,
			MediaType:      matcher.MediaMovie, // mock provider serves movies (§13.2).
			Title:          c.Title,
			Year:           c.Year,
		})
	}
	return out, nil
}
