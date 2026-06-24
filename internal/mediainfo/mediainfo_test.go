package mediainfo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestParseExtractsRuntimeResolutionStreamsAndTitle(t *testing.T) {
	raw := []byte(`{
		"format": {
			"format_name": "matroska,webm",
			"duration": "5400.000000",
			"tags": {"title": "Embedded Title"}
		},
		"streams": [
			{"index":0,"codec_type":"video","codec_name":"h264","width":1920,"height":1080},
			{"index":1,"codec_type":"audio","codec_name":"aac","tags":{"language":"jpn","title":"Japanese"}},
			{"index":2,"codec_type":"audio","codec_name":"ac3","tags":{"language":"eng"}},
			{"index":3,"codec_type":"subtitle","codec_name":"subrip","tags":{"language":"chi"}}
		]
	}`)
	got := parse(raw)
	if !got.OK {
		t.Fatalf("OK = false")
	}
	if got.RuntimeMinutes != 90 {
		t.Fatalf("runtime = %d, want 90", got.RuntimeMinutes)
	}
	if got.Width != 1920 || got.Height != 1080 {
		t.Fatalf("resolution = %dx%d", got.Width, got.Height)
	}
	if got.VideoCodec != "h264" {
		t.Fatalf("video codec = %q", got.VideoCodec)
	}
	if got.EmbeddedTitle != "Embedded Title" {
		t.Fatalf("title = %q", got.EmbeddedTitle)
	}
	if len(got.AudioStreams) != 2 {
		t.Fatalf("audio streams = %d", len(got.AudioStreams))
	}
	if got.AudioStreams[0].Language != "jpn" || got.AudioStreams[0].Title != "Japanese" {
		t.Fatalf("audio[0] = %#v", got.AudioStreams[0])
	}
	if len(got.SubtitleStreams) != 1 || got.SubtitleStreams[0].Language != "chi" {
		t.Fatalf("subtitle streams = %#v", got.SubtitleStreams)
	}
	if got.ProbeVersion != ProbeVersion {
		t.Fatalf("probe version = %q", got.ProbeVersion)
	}
}

func TestParseRoundsRuntime(t *testing.T) {
	raw := []byte(`{"format":{"duration":"100.0"}}`)
	if got := parse(raw); got.RuntimeMinutes != 2 { // 100s -> 1.67min -> rounds to 2
		t.Fatalf("runtime = %d, want 2", got.RuntimeMinutes)
	}
}

func TestParseMissingDurationStaysZero(t *testing.T) {
	raw := []byte(`{"format":{"format_name":"mp4"}}`)
	got := parse(raw)
	if !got.OK {
		t.Fatalf("OK = false")
	}
	if got.RuntimeMinutes != 0 || got.DurationSeconds != 0 {
		t.Fatalf("runtime/duration should be zero, got %d / %f", got.RuntimeMinutes, got.DurationSeconds)
	}
}

func TestParseGarbageDegradesGracefully(t *testing.T) {
	got := parse([]byte(`not json`))
	if got.OK {
		t.Fatalf("OK should be false for garbage payload")
	}
	if got.RuntimeMinutes != 0 || got.Width != 0 {
		t.Fatalf("expected zero values, got %#v", got)
	}
}

// TestProbeMissingFileDegradesGracefully ensures a probe of a nonexistent file
// never errors out the scan; it yields OK=false zero values.
func TestProbeMissingFileDegradesGracefully(t *testing.T) {
	if !Available() {
		t.Skip("ffprobe not available")
	}
	got := Probe(filepath.Join(t.TempDir(), "nope.mkv"))
	if got.OK {
		t.Fatalf("OK should be false for missing file")
	}
}

// TestProbeRealFile runs an end-to-end probe against a generated media file.
// It is guarded behind ffprobe AND ffmpeg availability.
func TestProbeRealFile(t *testing.T) {
	if !Available() {
		t.Skip("ffprobe not available")
	}
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not available to synthesize a test file")
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "sample.mkv")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// 2 seconds of 320x240 test video with a title tag.
	cmd := exec.CommandContext(ctx, ff,
		"-f", "lavfi", "-i", "testsrc=duration=2:size=320x240:rate=24",
		"-metadata", "title=Sample",
		"-y", out,
	)
	if err := cmd.Run(); err != nil {
		t.Skipf("ffmpeg failed to synthesize file: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Skipf("synthesized file missing: %v", err)
	}
	info := Probe(out)
	if !info.OK {
		t.Fatalf("probe of real file returned OK=false")
	}
	if info.Width != 320 || info.Height != 240 {
		t.Fatalf("resolution = %dx%d, want 320x240", info.Width, info.Height)
	}
	if info.EmbeddedTitle != "Sample" {
		t.Fatalf("embedded title = %q, want Sample", info.EmbeddedTitle)
	}
}
