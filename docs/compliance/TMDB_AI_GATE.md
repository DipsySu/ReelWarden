# COMPLIANCE-TMDB-AI Gate

Default status: `blocked`.

Runtime rule: when TMDB and AI are both enabled and this gate is not `accepted`,
ReelWarden must reject the configuration with `COMPLIANCE_PROVIDER_AI_COMBINATION_BLOCKED`.

This document is maintained per authority §13.7. It records the terms basis, risk, and
permitted/forbidden data flows for the gate.

## 1. Terms version / date

- Source: TMDB API Terms of Use — `https://www.themoviedb.org/api-terms-of-use`.
- Reviewed: 2026-06-24.
- **Verification status: PRELIMINARY.** The clause wording below was obtained via web
  search (two independent queries that agreed). The primary terms page could not be
  fetched directly (TLS/certificate error) and the Wayback copy was unreachable.
  **The exact current wording, section numbers, and effective date MUST be confirmed
  against the live page before any legal decision (e.g. unlocking the gate).**

## 2. Risk summary

TMDB's API Terms of Use contain explicit, **categorical** AI/ML use restrictions
(not conditioned on commercial vs non-commercial use). As surfaced:

- Prohibition on using the TMDB APIs / Content "in connection with ... interactive
  query-response system (including large language model (LLM), artificial intelligence,
  or any other machine learning based interactive query-response systems or chatbots)".
- Prohibition on using the TMDB APIs / Content "in connection with, including for
  training, a machine learning (ML) or artificial intelligence (AI) based Application".
- Prohibition on training/validating ML/AI systems with TMDB content, and on
  collecting/caching/providing datasets of TMDB content for that purpose.

Implication: feeding TMDB-returned content (titles, overviews, cast, IDs, raw payloads,
or TMDB-derived fields written into local NFO) into an AI/LLM prompt is, on the current
reading, prohibited. **"Non-commercial / open-source / BYOK / personal/self-hosted use"
do NOT exempt this** (authority §7.3, §0.2). The risk is a terms (contract) breach; the
realistic enforcement mechanism against an API-key holder is key revocation.

## 3. Clarification requests sent

None yet. Authority unlock path #1 (written clarification from TMDB) has not been
initiated. TMDB Talk has precedent threads on non-commercial ML use.

## 4. Responses received

None.

## 5. Decision

`COMPLIANCE-TMDB-AI` = `blocked` by default (unchanged). The gate is now backed by
documented (preliminary) terms findings rather than "pending review". No product
capability sends TMDB content to AI. The AI assistant is restricted to local-only data
(see §6). Re-evaluate only via authority §7.3 unlock paths: #1 written permission,
#2 formal legal review, #3 a provider whose terms allow the combination, #4 remove the
capability.

## 6. Allowed data flows

```text
TMDB API  -> deterministic code (parse / hard-constraint / score) -> human review -> plan
AI        <- local untrusted data ONLY: file name, parent dir name, relative dir,
             local NFO non-provider fields   (authority §7.2 AgentView, §7.4)
AI        -> normalization / media-type HYPOTHESES -> deterministic code
```

- AI may run **before** the API call to repair/normalize a local filename and propose a
  media-type hint (resolver rung R4, authority §14.9).
- AI never calls the Metadata Provider and never decides the final match (§7.1, §14.1).

## 7. Forbidden data flows

```text
TMDB content (title / original_title / overview / cast / images / raw JSON /
              TMDB-TVDB-Bangumi fields / TMDB-derived NFO fields)  -> AI / LLM     BLOCKED
AI  -> search_metadata_provider / read_provider_item / read_provider_cache         BLOCKED
TMDB + AI both enabled while gate != accepted                                      BLOCKED
```

Forbidden even for non-commercial / open-source / BYOK / personal/self-hosted use.
