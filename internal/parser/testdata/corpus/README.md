# §12.6 Parser Evaluation Corpus

This directory is the ground-truth corpus for the file-name parser evaluation
harness (authority §12.6). The harness lives in `internal/parser/corpus_test.go`
and computes **title-field** and **year-field** accuracy against these targets.

## What the ground truth is (and is not)

Every entry asserts what a **deterministic file-name parser** must extract from a
single file name. The ground truth is therefore *authorable* and *self-contained*:
it is a property of the string, not of any external database.

- `title` is the parser's `RawTitle` — the preserved display title region of the
  file name *before* normalization (§12.1/§12.3). It is NOT a TMDB/Provider title,
  NOT a translation, and NOT a normalized comparison key.
- `year` is the title-aware year (§12.4). `0`/absent means "the file name carries
  no year" (e.g. `1917.mkv`, `Spider-Man.mkv`). Such entries are excluded from the
  year-accuracy denominator (§12.6 measures year accuracy "on samples that have a
  year").
- `edition` is the §9.2 edition label (e.g. `Director's Cut`, `Extended Edition`)
  or empty. It is scored for visibility but is not part of the §12.6 thresholds.

No Provider / TMDB content appears anywhere in this corpus. The harness only ever
calls `parser.Parse` on a local file name. This keeps the corpus on the correct
side of the §7.1/§7.2 AI/Provider boundary: it is local, untrusted, authorable
data.

## Thresholds (§12.6)

- Title-field accuracy ≥ 95% (over all entries).
- Year-field accuracy ≥ 98% (over entries that declare a non-zero `year`).

The harness fails the build if either threshold is missed on the **development**
split. The **held-out** split is reported for visibility but its failures do not
gate, and — critically — it must never be inspected when tuning parser rules
(§12.6: "held-out not used for rule tuning pre-release").

## Layout

```
testdata/corpus/
├── README.md            (this file)
├── development/         scored + gating; safe to read while tuning rules
│   └── *.jsonl
└── heldout/             reported only; DO NOT read while tuning rules
    └── *.jsonl
```

Each `*.jsonl` file is one JSON object per line (blank lines and lines beginning
with `#` are ignored, so files can carry section headers as comments):

```json
{"filename": "Dune.2021.1080p.BluRay.x264-RARBG.mkv", "title": "Dune", "year": 2021, "edition": "", "note": "ascii + group"}
```

Fields:

| field      | required | meaning                                                            |
|------------|----------|--------------------------------------------------------------------|
| `filename` | yes      | the file name (or relative path) fed to `parser.Parse`             |
| `title`    | yes      | expected `RawTitle` (preserved display title, §12.1)               |
| `year`     | no       | expected title-aware year; omit or `0` = no year in the file name  |
| `edition`  | no       | expected edition label, or "" / omitted                            |
| `note`     | no       | human-facing description of what the entry exercises               |

## Regression entries

The development split begins with `00_regressions.jsonl`, which pins every
file-name shape called out in the code review. These are permanent: they encode
the exact failure scenarios (hyphenated titles not split as release groups,
leading bracket year recovery, single-letter title words not romanized, numeric
titles protected, fullwidth-digit titles preserved verbatim, CJK simp/trad/jp).
Do not delete them.

## Status: seed set, not yet the full 200

§12.6 requires a curated **200-entry** corpus (150 development / 50 held-out).
This directory currently seeds a smaller, diverse starter set (en / zh / ja / ko,
simplified + traditional, editions, release groups, technical tags, numeric and
fullwidth titles). Growing it to the full 150/50 curated set is **follow-on
work**. The split directories and the harness are already structured for that
target, so new entries can be dropped into `development/` or `heldout/` without
touching the harness. The held-out directory is physically separate so it is easy
to keep out of rule-tuning review.
