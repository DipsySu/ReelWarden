package naming

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/reelwarden/reelwarden/internal/store"
)

var unsafe = regexp.MustCompile(`[<>:"/\\|?*]+`)

func JellyfinPath(asset store.MediaAsset, c store.Candidate) string {
	title := sanitize(c.Title)
	year := ""
	if c.Year > 0 {
		year = fmt.Sprintf(" (%d)", c.Year)
	}
	ext := filepath.Ext(asset.RelativePath)
	file := title + year + ext
	return filepath.ToSlash(filepath.Join(title+year, file))
}
func sanitize(s string) string {
	s = unsafe.ReplaceAllString(s, " ")
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "Unknown Movie"
	}
	return s
}
