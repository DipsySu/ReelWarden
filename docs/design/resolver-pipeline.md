# Resolver Pipeline — Confidence-Routed Escalation (R0–R5)

Implementation contract for authority §14.9. Defines the per-file resolver, its data
model, and module boundaries so parallel implementation does not diverge on interfaces.
This document is subordinate to the authority baseline; on any conflict, the authority
doc (§12, §14, §7) wins.

## Compliance boundary (non-negotiable)

- AI consumes **local untrusted data only**: file name, parent dir name, relative dir,
  local NFO non-provider fields (authority §7.2, §7.4).
- AI never receives Provider/TMDB content, never calls the Provider, never decides the
  final match. AI emits **hypotheses** for deterministic code (§14.1, §7.1).
- TMDB + AI both enabled while `COMPLIANCE-TMDB-AI != accepted` → blocked (§7.3).

## Data model contract

`ParsedIdentity` (extends authority §9.2; supersedes the current `parser.Result`):

```go
type ParsedIdentity struct {
    RawTitle        string            // title region of the file name, pre-normalization
    NormalizedTitle string            // §12.3 normalization; comparison use only, never display
    ComparisonKeys  []string          // extra match keys, e.g. simplified/traditional fold (§12.3)
    Year            int               // 0 if absent; title-aware (§12.4), not "last 4 digits"
    Edition         string            // Director's Cut / Extended / Remux-as-edition (§9.2)
    ReleaseGroup    string            // stripped scene/release group
    TechnicalTags   []string          // §12.2 full set
    MediaTypeHint   string            // "" | movie | tv | tv_liveaction | ova | special
    ParentDirName   string            // local untrusted; type/title signal (R2)
    Hypotheses      []QueryHypothesis // one or more; ordered most-constrained first (§12.6)
    Confidence      float64           // heuristic, NOT a probability (§9.2)
    ParserVersion   string
    State           string            // parsed | unresolved
}

type QueryHypothesis struct {
    Title     string
    Year      int
    MediaType string // constrains the provider query (movie vs tv endpoint)
    Source    string // rule | parent_dir | romanized | comparison_key | ai_repair
}
```

`Evidence` / `score_band` / `rank_score` follow §14.8 verbatim — do not redefine.

## Rungs

Each rung takes the asset + accumulated `ParsedIdentity`, may emit `Evidence`, and returns
an exit decision: **stop** (band reached) or **escalate**. A file climbs only as far as
needed. Confidence is the router — file "kind" (managed vs raw) is discovered, not assumed.

| Rung | Input | Work | Exit |
| --- | --- | --- | --- |
| R0 | path, parent dir | preserve inputs (§12.1); normalize title (§12.3): Unicode NFC, fullwidth↔halfwidth, case fold, roman numerals, CJK punctuation, separators; build `ComparisonKeys` | always continue |
| R1 | file name, sibling `*.nfo` | detect embedded external IDs (`[tmdbid-…]`, `{imdb-tt…}`, `{tvdb-…}`) and NFO `<uniqueid>` (§14.2) | ID found → query by ID, expect **high** |
| R2 | media probe, tokens, parent dir | ffprobe runtime/resolution/audio+sub langs/embedded title; media-type tokens (剧场版/OVA/SP/真人/实写/Live Action/Drama/TV); parent-dir + sibling context; build constrained `Hypotheses` | continue to R3 |
| R3 | provider candidates (structured fields) | deterministic match §14.3–14.7: hard constraints → title evidence (max, not sum) → auxiliary (year/runtime/parent-dir) → conflict penalties → `rank_score` + `score_band` | `high` or `medium` → stop; `low` → escalate |
| R4 | local signals only | **local AI** repairs garbled filename (`低zhi商犯罪`→`低智商犯罪`) and proposes media-type hint; output appended to `Hypotheses`; re-run R2/R3 | resolved → stop; else escalate |
| R5 | all evidence | emit `unresolved` + ranked candidate hypotheses for human; never fabricate confidence (§12.6, §14.7) | terminal |

Bands (§14.7): `rank_score >= 0.95` high (preselect, still human-confirm); `0.80–0.95`
medium (require review); `< 0.80` low (escalate or `unresolved`). No auto-confirm in v0.1.1.

## TMDB deterministic query plan

TMDB remains a deterministic provider door. It is never queried by AI and its responses
are never sent to AI. The adapter must execute requests in this order:

1. Local exact IDs first:
   - local `tmdb` ID -> `/movie/{tmdb_id}` or `/tv/{tmdb_id}` based on `MediaTypeHint`;
   - local `imdb`, `tvdb`, or `wikidata` ID -> `/find/{external_id}` with
     `external_source=imdb_id|tvdb_id|wikidata_id`.
2. Title fallback only when needed:
   - movie-like hypotheses -> `/search/movie` with `query`, `language`, `region`,
     `primary_release_year`, `year`, `include_adult=false`, `page=1`;
   - tv-like hypotheses -> `/search/tv` with `query`, `language`,
     `first_air_date_year`, `year`, `include_adult=false`, `page=1`;
   - unknown type -> movie search first, tv search second.

The query planner is pure and testable (`internal/tmdb`); it emits request plans only,
not HTTP calls. Runtime/edition/language signals may further constrain future adapter
requests, but must remain deterministic and explainable.

## Module boundaries (for parallel implementation)

Foundation must land first; the rest depend on its interfaces.

- **Foundation (blocking):** `ParsedIdentity` + store schema/migration + resolver skeleton
  (routing, evidence accumulation, band thresholds). All other modules code against this.
- **R1 — id/nfo:** sidecar `.nfo` discovery in scanner + embedded-ID extraction in parser.
- **R2 — probe:** ffprobe wrapper (runtime/streams) + media-type token extractor + parent
  dir / sibling context.
- **R0/R3 inputs — normalization:** §12.3 normalizer + `ComparisonKeys` + release-group /
  edition extraction.
- **R3 — scorer:** §14.3–14.7 four-phase scoring + §14.8 evidence emission.
- **R4 — local-ai-repair:** local-only filename repair; input strictly per §7.2; emits
  hypotheses only. (Gated by the compliance boundary above.)
- **eval:** §12.6 corpus harness, 200 samples (150 dev / 50 held-out), title ≥95% /
  year ≥98%. Held-out not used for rule tuning pre-release.

## Invariants

- Title normalization never overwrites the display title (§12.3); keep raw + source.
- Do not strip digits that belong to the title for recall's sake (§12.6).
- Every rung emits evidence (§14.8) — explainability and the §14.6 "user previously
  rejected" penalty depend on it.
- Provider query is issued by deterministic code, never by AI.
