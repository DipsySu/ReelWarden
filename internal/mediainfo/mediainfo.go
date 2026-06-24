// Package mediainfo is a thin, fail-soft ffprobe wrapper for the resolver R2
// rung (authority §0.2, §7-probe spec, media_probes schema). It extracts
// runtime minutes, resolution, audio/subtitle language tracks and the embedded
// title tag from a local media file.
//
// It consumes a LOCAL file only and never touches the provider. ffprobe is
// optional: when the binary is absent, times out, or returns garbage, Probe
// returns zero values with OK=false and a nil error. A probe failure must
// NEVER block the local scan.
package mediainfo

import (
	"context"
	"encoding/json"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ProbeVersion identifies the probe extraction logic (stored as probe_version).
const ProbeVersion = "ffprobe-1"

// defaultTimeout bounds a single ffprobe invocation so a pathological file
// cannot stall the scan.
const defaultTimeout = 30 * time.Second

// Stream describes one audio or subtitle track. Language is the ffprobe tag
// (ISO-639-ish) when present; Title is the per-stream title tag.
type Stream struct {
	Index    int    `json:"index"`
	Codec    string `json:"codec,omitempty"`
	Language string `json:"language,omitempty"`
	Title    string `json:"title,omitempty"`
}

// Info is the normalized probe result. All fields are zero when OK is false.
type Info struct {
	OK              bool     `json:"ok"`               // false when ffprobe is unavailable or failed
	RuntimeMinutes  int      `json:"runtime_minutes"`  // rounded from container duration; 0 if unknown
	DurationSeconds float64  `json:"duration_seconds"` // raw container duration
	ContainerFormat string   `json:"container_format,omitempty"`
	Width           int      `json:"width,omitempty"`
	Height          int      `json:"height,omitempty"`
	VideoCodec      string   `json:"video_codec,omitempty"`
	EmbeddedTitle   string   `json:"embedded_title,omitempty"` // local untrusted; format-level title tag
	AudioStreams    []Stream `json:"audio_streams"`
	SubtitleStreams []Stream `json:"subtitle_streams"`
	ProbeVersion    string   `json:"probe_version"`
}

// Available reports whether an ffprobe binary is resolvable on PATH. Callers
// may use it to guard probe-dependent work or tests.
func Available() bool {
	_, err := exec.LookPath("ffprobe")
	return err == nil
}

// Probe runs ffprobe against path with the default timeout. It never returns a
// non-nil error for an absent or failing probe; instead Info.OK is false. The
// returned error is reserved for caller-cancellation surfaced via ctx in
// ProbeContext (Probe itself swallows all probe failures into OK=false).
func Probe(path string) Info {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	return ProbeContext(ctx, path)
}

// ProbeContext is Probe with a caller-supplied context (timeout/cancellation).
// It still degrades to OK=false rather than erroring on any ffprobe failure.
func ProbeContext(ctx context.Context, path string) Info {
	out := Info{ProbeVersion: ProbeVersion, AudioStreams: []Stream{}, SubtitleStreams: []Stream{}}
	bin, err := exec.LookPath("ffprobe")
	if err != nil {
		return out // ffprobe absent -> graceful zero value
	}
	cmd := exec.CommandContext(ctx, bin,
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path,
	)
	raw, err := cmd.Output()
	if err != nil {
		return out // ffprobe failed/timed out -> graceful zero value
	}
	return parse(raw)
}

// ffprobe JSON shapes (only the fields we use).
type rawProbe struct {
	Format struct {
		FormatName string            `json:"format_name"`
		Duration   string            `json:"duration"`
		Tags       map[string]string `json:"tags"`
	} `json:"format"`
	Streams []rawStream `json:"streams"`
}

type rawStream struct {
	Index     int               `json:"index"`
	CodecType string            `json:"codec_type"`
	CodecName string            `json:"codec_name"`
	Width     int               `json:"width"`
	Height    int               `json:"height"`
	Tags      map[string]string `json:"tags"`
}

// parse is the pure JSON-to-Info transform, split out so it is unit-testable
// without ffprobe present.
func parse(raw []byte) Info {
	out := Info{ProbeVersion: ProbeVersion, AudioStreams: []Stream{}, SubtitleStreams: []Stream{}}
	var p rawProbe
	if err := json.Unmarshal(raw, &p); err != nil {
		return out // unparseable payload -> graceful zero value
	}
	out.OK = true
	out.ContainerFormat = p.Format.FormatName
	out.EmbeddedTitle = tag(p.Format.Tags, "title")
	if d, err := strconv.ParseFloat(strings.TrimSpace(p.Format.Duration), 64); err == nil && d > 0 {
		out.DurationSeconds = d
		out.RuntimeMinutes = int(d/60 + 0.5)
	}
	for _, s := range p.Streams {
		switch s.CodecType {
		case "video":
			// First video stream defines the asset resolution/codec.
			if out.Width == 0 && out.Height == 0 {
				out.Width = s.Width
				out.Height = s.Height
				out.VideoCodec = s.CodecName
			}
		case "audio":
			out.AudioStreams = append(out.AudioStreams, stream(s))
		case "subtitle":
			out.SubtitleStreams = append(out.SubtitleStreams, stream(s))
		}
	}
	return out
}

func stream(s rawStream) Stream {
	return Stream{
		Index:    s.Index,
		Codec:    s.CodecName,
		Language: tag(s.Tags, "language"),
		Title:    tag(s.Tags, "title"),
	}
}

// tag does a case-insensitive lookup of a ffprobe tag key.
func tag(tags map[string]string, key string) string {
	if tags == nil {
		return ""
	}
	if v, ok := tags[key]; ok {
		return v
	}
	for k, v := range tags {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}
