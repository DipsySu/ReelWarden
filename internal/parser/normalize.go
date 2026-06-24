package parser

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// normalize.go implements authority §12.3 title normalization. The output is used
// for candidate retrieval and scoring ONLY; it must never overwrite the raw/display
// title (§12.3). The pipeline keeps the original RawTitle and records the source of
// every derived comparison key. Implemented with stdlib only (no x/text dependency).

// NormalizeTitle applies the §12.3 normalization chain to a raw title region and
// returns the NormalizedTitle plus the extra ComparisonKeys (e.g. a simplified/
// traditional fold key). The returned NormalizedTitle is for comparison only.
//
// Chain (§12.3):
//   - Unicode normalization (canonical composition over the common precomposed set)
//   - fullwidth <-> halfwidth folding
//   - CJK / zh punctuation unification
//   - separator unification (., _, etc. -> space) and whitespace folding
//   - simple roman-numeral compatibility (II -> 2, ...)
//   - case folding
//   - digits and CJK characters preserved; no translation; no simp/trad overwrite
func NormalizeTitle(raw string) (normalized string, comparisonKeys []string) {
	s := normalizeBase(raw)
	if s == "" {
		return "", nil
	}

	keys := keySet{}
	// The fold key: simplified/traditional unified. Always derived from the
	// normalized form; recorded as a separate key, never overwriting the value.
	if fold := simplifyFold(s); fold != "" && fold != s {
		keys.add(fold, "simp_trad_fold")
	}
	// A space-stripped key helps match CJK titles where releases insert or drop
	// spaces arbitrarily (e.g. "沙丘 沙丘" vs "沙丘沙丘"). Restricted to titles that
	// contain Han characters: for ASCII titles word spacing is meaningful, so a
	// spaceless key would only add noise.
	if containsHan(s) {
		if compact := stripInnerSpaces(s); compact != "" && compact != s {
			keys.add(compact, "spaceless")
		}
	}
	return s, keys.values()
}

// normalizeBase runs the deterministic §12.3 chain that produces the comparison
// value (everything except the extra fold/spaceless keys).
func normalizeBase(raw string) string {
	s := canonicalCompose(raw) // Unicode normalization (NFC-style recomposition)
	s = foldWidth(s)           // fullwidth <-> halfwidth
	s = unifyPunct(s)          // CJK / zh punctuation -> ASCII equivalents
	s = unifySeparators(s)     // . _ / etc. -> space
	s = romanizeNumerals(s)    // II -> 2 (simple roman numerals)
	s = caseFold(s)            // lower-case fold
	s = collapseSpaces(s)      // fold runs of whitespace, trim
	return s
}

// ----- Unicode normalization (NFC-style canonical recomposition) -----

// canonicalCompose recomposes the common precomposed Latin/diacritic sequences and
// drops stray combining marks. Go's stdlib has no NFC; for media-file titles the
// realistic decomposed input is base-letter + combining diacritic (NFD), so we
// recombine those into their precomposed forms and strip any leftover marks. CJK
// text has no combining marks here and passes through unchanged.
func canonicalCompose(s string) string {
	if !hasCombining(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if i+1 < len(rs) && unicode.Is(unicode.Mn, rs[i+1]) {
			if c, ok := composeRune(r, rs[i+1]); ok {
				b.WriteRune(c)
				i++ // consumed the combining mark
				continue
			}
		}
		if unicode.Is(unicode.Mn, r) {
			continue // stray combining mark we cannot compose: drop it
		}
		b.WriteRune(r)
	}
	return b.String()
}

func hasCombining(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Mn, r) {
			return true
		}
	}
	return false
}

// composeRune combines a base rune with a single combining mark into its
// precomposed form for the diacritics common in Latin-script film titles.
func composeRune(base, mark rune) (rune, bool) {
	table, ok := combiningTables[mark]
	if !ok {
		return 0, false
	}
	c, ok := table[base]
	return c, ok
}

const (
	markGrave     = '̀'
	markAcute     = '́'
	markCircumfle = '̂'
	markTilde     = '̃'
	markDiaeresis = '̈'
	markRingAbove = '̊'
	markCedilla   = '̧'
)

var combiningTables = map[rune]map[rune]rune{
	markGrave:     {'a': 'à', 'e': 'è', 'i': 'ì', 'o': 'ò', 'u': 'ù', 'A': 'À', 'E': 'È', 'I': 'Ì', 'O': 'Ò', 'U': 'Ù'},
	markAcute:     {'a': 'á', 'e': 'é', 'i': 'í', 'o': 'ó', 'u': 'ú', 'y': 'ý', 'A': 'Á', 'E': 'É', 'I': 'Í', 'O': 'Ó', 'U': 'Ú', 'Y': 'Ý'},
	markCircumfle: {'a': 'â', 'e': 'ê', 'i': 'î', 'o': 'ô', 'u': 'û', 'A': 'Â', 'E': 'Ê', 'I': 'Î', 'O': 'Ô', 'U': 'Û'},
	markTilde:     {'a': 'ã', 'n': 'ñ', 'o': 'õ', 'A': 'Ã', 'N': 'Ñ', 'O': 'Õ'},
	markDiaeresis: {'a': 'ä', 'e': 'ë', 'i': 'ï', 'o': 'ö', 'u': 'ü', 'y': 'ÿ', 'A': 'Ä', 'E': 'Ë', 'I': 'Ï', 'O': 'Ö', 'U': 'Ü'},
	markRingAbove: {'a': 'å', 'A': 'Å'},
	markCedilla:   {'c': 'ç', 'C': 'Ç'},
}

// ----- fullwidth <-> halfwidth folding -----

// foldWidth maps fullwidth ASCII variants (U+FF01..U+FF5E) to their halfwidth
// ASCII forms and the ideographic space (U+3000) to a regular space. Halfwidth
// katakana etc. are left untouched (not relevant to title comparison here).
func foldWidth(s string) string {
	if !needsWidthFold(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 0xFF01 && r <= 0xFF5E:
			b.WriteRune(r - 0xFEE0) // shift fullwidth ASCII into ASCII range
		case r == 0x3000:
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func needsWidthFold(s string) bool {
	for _, r := range s {
		if (r >= 0xFF01 && r <= 0xFF5E) || r == 0x3000 {
			return true
		}
	}
	return false
}

// ----- CJK / zh punctuation unification -----

// punctFold maps CJK and "smart" punctuation to ASCII equivalents so that titles
// using 「」、，。 etc. compare equal to their ASCII-punctuation forms.
var punctFold = map[rune]rune{
	'：': ':', '；': ';', '，': ',', '。': '.', '、': ' ',
	'！': '!', '？': '?', '「': ' ', '」': ' ', '『': ' ', '』': ' ',
	'（': '(', '）': ')', '【': ' ', '】': ' ', '《': ' ', '》': ' ',
	'〈': ' ', '〉': ' ', '·': ' ', '・': ' ', '～': '~',
	'“': '"', '”': '"', '‘': '\'', '’': '\'',
	'—': '-', '–': '-', '―': '-', '－': '-', '−': '-',
	'…': ' ',
}

func unifyPunct(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if rep, ok := punctFold[r]; ok {
			b.WriteRune(rep)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ----- separator unification -----

// unifySeparators turns the §12.3 separators into spaces. Dots, underscores,
// slashes and bracketing become spaces; CJK characters and digits are preserved.
func unifySeparators(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '.', '_', '/', '\\', '|', '[', ']', '(', ')', '{', '}', '+', '~':
			return ' '
		}
		return r
	}, s)
}

// ----- simple roman numerals -----

var romanValues = map[string]int{
	"ii": 2, "iii": 3, "iv": 4, "v": 5, "vi": 6, "vii": 7, "viii": 8, "ix": 9,
	"x": 10, "xi": 11, "xii": 12, "xiii": 13,
}

// romanizeNumerals rewrites standalone roman numerals (II..XIII) into Arabic
// digits so "Rocky II" and "Rocky 2" compare equal. "I" is intentionally excluded
// (too ambiguous with the pronoun / single letters). Case-insensitive.
func romanizeNumerals(s string) string {
	if s == "" {
		return s
	}
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ' ' })
	if len(fields) == 0 {
		return s
	}
	changed := false
	for i, f := range fields {
		low := strings.ToLower(f)
		if v, ok := romanValues[low]; ok {
			fields[i] = itoa(v)
			changed = true
		}
	}
	if !changed {
		return s
	}
	// Preserve leading/trailing spaces semantics by rejoining on single spaces;
	// collapseSpaces later normalizes anyway.
	return strings.Join(fields, " ")
}

// ----- case folding -----

func caseFold(s string) string {
	// strings.ToLower applies Unicode simple case folding; CJK is unaffected.
	return strings.ToLower(s)
}

// ----- whitespace handling -----

func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func stripInnerSpaces(s string) string {
	if !strings.ContainsRune(s, ' ') {
		return s
	}
	return strings.ReplaceAll(s, " ", "")
}

// ----- simplified / traditional fold -----

// simplifyFold maps each traditional Han character it knows to its simplified
// canonical form, producing a comparison key that unifies simp/trad variants of a
// title (§12.3). The mapping is intentionally one-directional (trad -> simp) so the
// key is canonical; characters not in the table pass through unchanged. This NEVER
// overwrites the stored title — it only produces an additional comparison key.
func simplifyFold(s string) string {
	if !containsHan(s) {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	folded := false
	for _, r := range s {
		if simp, ok := tradToSimp[r]; ok {
			b.WriteRune(simp)
			folded = true
			continue
		}
		b.WriteRune(r)
	}
	if !folded {
		return s // already simplified (or no known trad chars): key == value
	}
	return b.String()
}

func containsHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// tradToSimp is a curated traditional->simplified map covering characters that
// recur in film titles. It is not exhaustive; unknown characters pass through, so
// the worst case is a missed extra key, never a corrupted title.
var tradToSimp = map[rune]rune{
	'無': '无', '間': '间', '臥': '卧', '龍': '龙',
	'愛': '爱', '國': '国', '夢': '梦', '兒': '儿', '門': '门', '車': '车', '馬': '马',
	'風': '风', '雲': '云', '電': '电', '視': '视', '劇': '剧', '場': '场',
	'戰': '战', '爭': '争', '殺': '杀', '機': '机', '槍': '枪', '醫': '医', '師': '师',
	'與': '与', '從': '从', '會': '会', '來': '来', '後': '后', '時': '时',
	'這': '这', '個': '个', '們': '们', '黨': '党', '邊': '边', '飛': '飞', '島': '岛',
	'歲': '岁', '萬': '万', '裡': '里', '裏': '里', '頭': '头', '臺': '台', '灣': '湾',
	'學': '学', '習': '习', '書': '书', '畫': '画', '長': '长', '開': '开', '關': '关',
	'種': '种', '陽': '阳', '陰': '阴', '聲': '声', '東': '东', '萊': '莱', '塢': '坞',
	'證': '证', '據': '据', '懸': '悬', '驚': '惊', '聯': '联',
	'復': '复', '俠': '侠', '鐵': '铁',
	'歸': '归', '魔': '魔', '獸': '兽', '鳥': '鸟', '鬥': '斗',
	'單': '单', '雙': '双', '對': '对', '紅': '红', '綠': '绿', '藍': '蓝', '橋': '桥',
	'戀': '恋', '純': '纯', '潔': '洁', '麗': '丽', '麼': '么', '見': '见', '觀': '观',
}

// keySet collects derived comparison keys, de-duplicating while remembering the
// derivation source for explainability. The Source labels match the documented
// ParsedIdentity hypothesis sources (e.g. "comparison_key").
type keySet struct {
	vals  []string
	seen  map[string]struct{}
	srcOf map[string]string
}

func (k *keySet) add(v, source string) {
	if v == "" {
		return
	}
	if k.seen == nil {
		k.seen = map[string]struct{}{}
		k.srcOf = map[string]string{}
	}
	if _, ok := k.seen[v]; ok {
		return
	}
	k.seen[v] = struct{}{}
	k.srcOf[v] = source
	k.vals = append(k.vals, v)
}

func (k *keySet) values() []string {
	if len(k.vals) == 0 {
		return nil
	}
	out := append([]string(nil), k.vals...)
	sort.Strings(out)
	return out
}

// itoa is a tiny dependency-free int->string for the roman-numeral rewrite.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// utf8Len is a small helper used by tests/edition logic to reason about rune width
// without importing utf8 everywhere.
func utf8Len(s string) int { return utf8.RuneCountInString(s) }
