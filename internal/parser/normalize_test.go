package parser

import (
	"reflect"
	"testing"
)

func TestNormalizeTitle(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantNorm string
		wantKeys []string // sorted; nil if none expected
	}{
		{"ascii lower + collapse", "The Matrix", "the matrix", nil},
		{"ascii has no spaceless key", "the  matrix", "the matrix", nil},
		{"fullwidth latin folds", "Ｄｕｎｅ", "dune", nil},
		{"fullwidth digits fold, ascii so no extra key", "Ｂｌａｄｅ２０４９", "blade2049", nil},
		{"ideographic space folds, ascii so no spaceless", "Ｄｕｎｅ　Ｐａｒｔ", "dune part", nil},
		{"roman numeral II -> 2, ascii so no extra key", "Rocky II", "rocky 2", nil},
		{"roman numeral bare I not converted", "I Am Legend", "i am legend", nil},
		{"roman IV folds", "Rambo IV", "rambo 4", nil},
		{"cjk punctuation unify, value is the key", "无间道：终极无间", "无间道:终极无间", nil},
		{"cjk no combine, simp passthrough", "沙丘", "沙丘", nil},
		{"traditional folds to simplified key", "無間道", "無間道", []string{"无间道"}},
		{"traditional crouching tiger", "臥虎藏龍", "臥虎藏龍", []string{"卧虎藏龙"}},
		{"already simplified, no extra key", "卧虎藏龙", "卧虎藏龙", nil},
		{"han with inner space gets spaceless key", "沙丘 前传", "沙丘 前传", []string{"沙丘前传"}},
		{"precomposed diacritic lowercases", "Amélie", "amélie", nil},
		{"smart quotes unify", "“Heat”", "\"heat\"", nil},
		{"empty stays empty", "   ", "", nil},
		{"digits preserved", "2046", "2046", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNorm, gotKeys := NormalizeTitle(tt.in)
			if gotNorm != tt.wantNorm {
				t.Errorf("norm = %q, want %q", gotNorm, tt.wantNorm)
			}
			if len(tt.wantKeys) == 0 && len(gotKeys) == 0 {
				return
			}
			if !reflect.DeepEqual(gotKeys, tt.wantKeys) {
				t.Errorf("keys = %v, want %v", gotKeys, tt.wantKeys)
			}
		})
	}
}

// TestCanonicalComposeNFD verifies the Unicode normalization step recomposes a
// genuinely decomposed (NFD) sequence: base 'e' + U+0301 combining acute -> precomposed.
func TestCanonicalComposeNFD(t *testing.T) {
	const combiningAcute = '́'
	nfd := "Am" + string([]rune{'e', combiningAcute}) + "lie"
	if nfd == "Amélie" {
		t.Fatal("test bug: nfd input is not actually decomposed")
	}
	gotNorm, _ := NormalizeTitle(nfd)
	want := "am" + string('é') + "lie" // precomposed é, lowercased
	if gotNorm != want {
		t.Fatalf("NFD input not recomposed+lowered: got %q want %q", gotNorm, want)
	}
	// Composed and decomposed inputs must normalize identically.
	composed, _ := NormalizeTitle("Am" + string('É') + "lie") // É precomposed
	if gotNorm != composed {
		t.Fatalf("NFD %q and NFC %q normalize differently", gotNorm, composed)
	}
}

func TestSimplifyFoldCanonical(t *testing.T) {
	// Traditional and simplified writings of the same title must produce an equal
	// comparison key (the simp/trad fold key), so they collide at match time.
	cases := [][2]string{
		{"無間道", "无间道"},
		{"臥虎藏龍", "卧虎藏龙"},
	}
	for _, c := range cases {
		_, tradKeys := NormalizeTitle(c[0])
		simpNorm, _ := NormalizeTitle(c[1])
		found := false
		for _, k := range tradKeys {
			if k == simpNorm {
				found = true
			}
		}
		if !found {
			t.Errorf("trad %q fold keys %v do not include simplified %q", c[0], tradKeys, simpNorm)
		}
	}
}

// TestNormalizeDoesNotMutateRaw guards §12.3: the normalized title keeps the CJK
// characters verbatim (no auto simp/trad conversion of the value); only the extra
// key is simplified.
func TestNormalizeDoesNotMutateRaw(t *testing.T) {
	norm, keys := NormalizeTitle("無間道")
	if norm != "無間道" {
		t.Fatalf("normalized title was rewritten to %q; must keep CJK characters verbatim", norm)
	}
	if len(keys) == 0 || keys[0] != "无间道" {
		t.Fatalf("expected a simplified fold key 无间道, got %v", keys)
	}
}

func TestFoldWidth(t *testing.T) {
	if got := foldWidth("ＡＢＣ１２３！"); got != "ABC123!" {
		t.Errorf("foldWidth = %q, want ABC123!", got)
	}
	if got := foldWidth("plain"); got != "plain" {
		t.Errorf("foldWidth should pass ASCII unchanged, got %q", got)
	}
}

func TestRomanizeNumerals(t *testing.T) {
	tests := map[string]string{
		"rocky ii":      "rocky 2",
		"part iii here": "part 3 here",
		"i robot":       "i robot", // bare I is not converted (ambiguous)
		"no romans":     "no romans",
		"xii monkeys":   "12 monkeys",
	}
	for in, want := range tests {
		if got := romanizeNumerals(in); got != want {
			t.Errorf("romanizeNumerals(%q) = %q, want %q", in, got, want)
		}
	}
}
