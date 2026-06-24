package scanner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/reelwarden/reelwarden/internal/parser"
	"github.com/reelwarden/reelwarden/internal/store"
)

var videoExt = map[string]bool{".mkv": true, ".mp4": true, ".avi": true, ".mov": true, ".m4v": true, ".wmv": true, ".ts": true}

type Result struct {
	Scanned int                `json:"scanned"`
	Assets  []store.MediaAsset `json:"assets"`
}

func Scan(ctx context.Context, st *store.Store, root store.LibraryRoot) (Result, error) {
	var res Result
	err := filepath.WalkDir(root.Path, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if d.IsDir() {
			return nil
		}
		if !videoExt[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(root.Path, path)
		if err != nil {
			return nil
		}
		parsed := parser.ParsePath(rel)
		a := st.UpsertAsset(store.MediaAsset{LibraryRootID: root.ID, RelativePath: filepath.ToSlash(rel), SizeBytes: info.Size(), ModifiedAt: info.ModTime().UTC().Format(time.RFC3339), ScanState: "scanned", ParsedTitle: parsed.Title, ParsedYear: parsed.Year, MatchState: "needs_review"})
		res.Scanned++
		res.Assets = append(res.Assets, a)
		return nil
	})
	return res, err
}
