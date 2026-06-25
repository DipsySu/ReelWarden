package mediatype

import "testing"

func TestHint(t *testing.T) {
	cases := []struct {
		name      string
		fileName  string
		parentDir string
		want      string
	}{
		{"plain movie token", "Some.Anime.Movie.2019.1080p.mkv", "", HintMovie},
		{"cjk gekijouban", "名侦探柯南 剧场版.mkv", "", HintMovie},
		{"cjk gekijouban traditional", "某作品 劇場版.mkv", "", HintMovie},
		{"ova", "Some.Anime.OVA.01.mkv", "", HintOVA},
		{"oad", "Some.Anime.OAD.mkv", "", HintOVA},
		{"special long", "Some.Anime.Special.mkv", "", HintSpecial},
		{"special cjk", "某番 特别篇.mkv", "", HintSpecial},
		{"special sp standalone", "Show.SP.01.mkv", "", HintSpecial},
		{"sp not standalone stays none", "Crispy.2020.mkv", "", HintNone},
		{"tv drama", "Some.Show.Drama.S01.mkv", "", HintTV},
		// Regression: "season"/"series" must be token-bounded and only count in
		// the constrained "<marker> <number>" trailing form. Unbounded
		// strings.Contains fired on substrings inside ordinary title words
		// ("Seasoning") and on title-leading words ("Series 7 The Contenders").
		{"season substring in seasoning is not tv", "The Seasoning House", "", HintNone},
		{"series leading a film title is not tv", "Series 7 The Contenders", "", HintNone},
		{"season number is tv", "Some.Show.Season.1", "", HintTV},
		{"series number is tv", "Some Show Series 3", "", HintTV},
		{"sxx standalone is tv", "Some.Show.S01.E02.mkv", "", HintTV},
		{"tv cjk dianshiban", "进击的巨人 电视版.mkv", "", HintTV},
		{"tv liveaction cjk", "进击的巨人 真人版 电视剧.mkv", "进击的巨人 真人版", HintTVLiveAction},
		{"tv liveaction english", "Attack.on.Titan.Live.Action.Drama.mkv", "", HintTVLiveAction},
		{"liveaction movie stays movie", "进击的巨人 真人版 剧场版.mkv", "", HintMovie},
		{"parent dir signal", "ep01.mkv", "进击的巨人 真人版 电视剧", HintTVLiveAction},
		{"ova beats movie", "Show.剧场版.OVA.mkv", "", HintOVA},
		{"none", "Random.File.2021.1080p.mkv", "", HintNone},
		{"fullwidth separators", "某作品　剧场版．mkv", "", HintMovie},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Hint(c.fileName, c.parentDir)
			if got != c.want {
				t.Fatalf("Hint(%q,%q) = %q, want %q", c.fileName, c.parentDir, got, c.want)
			}
		})
	}
}

func TestHintEmptyInput(t *testing.T) {
	if got := Hint("", ""); got != HintNone {
		t.Fatalf("empty = %q, want none", got)
	}
}
