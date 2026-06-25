package parser

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// This file is the §12.6 parser-evaluation harness. It loads the authorable golden
// corpus under testdata/corpus/{development,heldout}, runs the deterministic
// parser.Parse over each file name, and measures title-field and year-field
// accuracy against the corpus targets.
//
// The ground truth is about FILE-NAME PARSING, not external TMDB facts: each entry
// asserts what RawTitle / Year / Edition the parser must extract from a single file
// name. No Provider content is involved — the harness only ever calls Parse on a
// local string, which keeps the corpus on the correct side of the §7.1/§7.2 AI /
// Provider boundary.
//
// §12.6 thresholds:
//   - title-field accuracy >= 95% (over all entries)
//   - year-field accuracy  >= 98% (over entries that declare a non-zero year)
//
// The development split GATES the build. The held-out split is reported for
// visibility only and must never be inspected when tuning parser rules
// (§12.6: "held-out not used for rule tuning pre-release"). See testdata/corpus/README.md.
const (
	titleAccuracyThreshold = 0.95
	yearAccuracyThreshold  = 0.98
)

// corpusEntry is one golden record (one JSON object per line). Year/Edition are
// optional; an absent or zero Year means "this file name carries no year" and the
// entry is excluded from the year-accuracy denominator (§12.4/§12.6).
type corpusEntry struct {
	Filename string `json:"filename"`
	Title    string `json:"title"`
	Year     int    `json:"year"`
	Edition  string `json:"edition"`
	Note     string `json:"note"`

	// source file + line, for actionable failure messages.
	srcFile string
	srcLine int
}

// loadCorpus reads every *.jsonl file under dir. Blank lines and lines beginning
// with '#' (after trimming) are ignored so files can carry comment headers.
func loadCorpus(t *testing.T, dir string) []corpusEntry {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		t.Fatalf("glob %s: %v", dir, err)
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		t.Fatalf("no *.jsonl corpus files under %s", dir)
	}
	var entries []corpusEntry
	for _, path := range matches {
		f, err := os.Open(path)
		if err != nil {
			t.Fatalf("open %s: %v", path, err)
		}
		base := filepath.Base(path)
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		lineNo := 0
		for sc.Scan() {
			lineNo++
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			var e corpusEntry
			if err := json.Unmarshal([]byte(line), &e); err != nil {
				f.Close()
				t.Fatalf("%s:%d: invalid json: %v", base, lineNo, err)
			}
			if e.Filename == "" {
				f.Close()
				t.Fatalf("%s:%d: entry has empty filename", base, lineNo)
			}
			e.srcFile = base
			e.srcLine = lineNo
			entries = append(entries, e)
		}
		if err := sc.Err(); err != nil {
			f.Close()
			t.Fatalf("scan %s: %v", base, err)
		}
		f.Close()
	}
	return entries
}

// scoreResult holds the accuracy tallies for one split.
type scoreResult struct {
	total       int
	titleOK     int
	yearSamples int // entries that declare a non-zero year
	yearOK      int
}

func (s scoreResult) titleAccuracy() float64 {
	if s.total == 0 {
		return 0
	}
	return float64(s.titleOK) / float64(s.total)
}

func (s scoreResult) yearAccuracy() float64 {
	if s.yearSamples == 0 {
		return 1 // vacuously satisfied; no year-bearing samples to score
	}
	return float64(s.yearOK) / float64(s.yearSamples)
}

// scoreSplit runs Parse over every entry and tallies title/year correctness. Each
// mismatch is logged with its source location so a failure is actionable, but only
// the caller decides whether mismatches gate the build (dev gates; held-out does not).
func scoreSplit(t *testing.T, entries []corpusEntry) scoreResult {
	t.Helper()
	var s scoreResult
	for _, e := range entries {
		// parentDir is intentionally empty: the corpus scores file-name parsing in
		// isolation. Path-shaped filenames still exercise the Base/Dir handling.
		id := Parse(e.Filename, "")
		s.total++

		if id.RawTitle == e.Title {
			s.titleOK++
		} else {
			t.Logf("TITLE mismatch %s:%d %q -> got %q, want %q (%s)",
				e.srcFile, e.srcLine, e.Filename, id.RawTitle, e.Title, e.Note)
		}

		if e.Year != 0 {
			s.yearSamples++
			if id.Year == e.Year {
				s.yearOK++
			} else {
				t.Logf("YEAR mismatch %s:%d %q -> got %d, want %d (%s)",
					e.srcFile, e.srcLine, e.Filename, id.Year, e.Year, e.Note)
			}
		}
	}
	return s
}

func corpusDir(split string) string {
	return filepath.Join("testdata", "corpus", split)
}

// TestParserCorpusDevelopment is the gating §12.6 evaluation: it asserts the
// development split meets the title (>=95%) and year (>=98%) accuracy thresholds.
// A regression in any rule that pushes accuracy below threshold fails the build.
func TestParserCorpusDevelopment(t *testing.T) {
	entries := loadCorpus(t, corpusDir("development"))
	s := scoreSplit(t, entries)

	t.Logf("development: %d entries | title %.2f%% (%d/%d) | year %.2f%% (%d/%d year-bearing)",
		s.total,
		s.titleAccuracy()*100, s.titleOK, s.total,
		s.yearAccuracy()*100, s.yearOK, s.yearSamples)

	if got := s.titleAccuracy(); got < titleAccuracyThreshold {
		t.Errorf("development title accuracy %.4f below §12.6 threshold %.2f", got, titleAccuracyThreshold)
	}
	if got := s.yearAccuracy(); got < yearAccuracyThreshold {
		t.Errorf("development year accuracy %.4f below §12.6 threshold %.2f", got, yearAccuracyThreshold)
	}
}

// TestParserCorpusHeldOut reports held-out accuracy for visibility WITHOUT gating
// the build (§12.6: held-out must not drive rule tuning pre-release). It still fails
// if the split cannot be loaded, guaranteeing the held-out set stays wired up.
func TestParserCorpusHeldOut(t *testing.T) {
	entries := loadCorpus(t, corpusDir("heldout"))
	s := scoreSplit(t, entries)
	t.Logf("held-out (report only): %d entries | title %.2f%% (%d/%d) | year %.2f%% (%d/%d year-bearing)",
		s.total,
		s.titleAccuracy()*100, s.titleOK, s.total,
		s.yearAccuracy()*100, s.yearOK, s.yearSamples)
}

// TestParserCorpusRegressionsPresent guards that the permanent review-repro entries
// stay in the development corpus. These encode the exact failure scenarios from the
// code review; deleting them must break the build, not silently drop coverage.
func TestParserCorpusRegressionsPresent(t *testing.T) {
	entries := loadCorpus(t, corpusDir("development"))
	have := make(map[string]bool, len(entries))
	for _, e := range entries {
		have[e.Filename] = true
	}
	required := []string{
		"Spider-Man.mkv",
		"X-Men.mkv",
		"Mission-Impossible.mkv",
		"DC.League.of.Super-Pets.2022.1080p.BluRay.x264.mkv",
		"V.for.Vendetta.2005.1080p.BluRay.mkv",
		"Malcolm.X.1992.mkv",
		"[2021] The Northman.mkv",
		"赌侠２.1991.BluRay.mkv",
		"Ｄｕｎｅ　２０２１.mkv",
		"Blade Runner 2049 (2017).mkv",
		"2046.2004.1080p.BluRay.mkv",
		"1917.mkv",
		"The.Seasoning.House.2012.1080p.BluRay.x264.mkv",
	}
	for _, fn := range required {
		if !have[fn] {
			t.Errorf("required regression entry missing from development corpus: %q", fn)
		}
	}
}
