package parser

import (
	"path/filepath"
	"regexp"
	"strings"
)

type Result struct {
	Title string
	Year  int
	Tags  []string
}

var yearRE = regexp.MustCompile(`(?:^|[^0-9])((?:19|20)[0-9]{2})(?:[^0-9]|$)`)
var tagRE = regexp.MustCompile(`(?i)\b(2160p|1080p|720p|hdr|dv|bluray|webrip|web-dl|x264|x265|h\.264|h\.265|remux)\b`)

func ParsePath(rel string) Result {
	base := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
	year := 0
	if m := yearRE.FindStringSubmatch(base); len(m) > 1 {
		year = atoi(m[1])
		base = strings.Replace(base, m[1], " ", 1)
	}
	tags := tagRE.FindAllString(base, -1)
	cleaned := tagRE.ReplaceAllString(base, " ")
	cleaned = strings.NewReplacer(".", " ", "_", " ", "-", " ", "[", " ", "]", " ", "(", " ", ")", " ").Replace(cleaned)
	fields := strings.Fields(cleaned)
	return Result{Title: strings.Join(fields, " "), Year: year, Tags: tags}
}
func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return n
		}
		n = n*10 + int(r-'0')
	}
	return n
}
