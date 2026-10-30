package main

import (
	"regexp"
	"strings"
	"unicode"
)

// maxSlugLen обмежує довжину текстової частини гілки (обрізаємо по межі слова).
const maxSlugLen = 50

// Транслітерація української (спрощена, читабельна) + російські літери.
var translit = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "h", 'ґ': "g", 'д': "d", 'е': "e",
	'є': "ye", 'ж': "zh", 'з': "z", 'и': "y", 'і': "i", 'ї': "yi", 'й': "y",
	'к': "k", 'л': "l", 'м': "m", 'н': "n", 'о': "o", 'п': "p", 'р': "r",
	'с': "s", 'т': "t", 'у': "u", 'ф': "f", 'х': "kh", 'ц': "ts", 'ч': "ch",
	'ш': "sh", 'щ': "shch", 'ь': "", 'ю': "yu", 'я': "ya",
	'ы': "y", 'э': "e", 'ё': "yo", 'ъ': "",
	// апострофи всередині слова просто прибираємо: "п'ять" -> "pyat"
	'\'': "", '’': "", 'ʼ': "", '`': "",
}

// Відмінності російської транслітерації від української.
var translitRu = map[rune]string{'и': "i", 'г': "g", 'й': "y"}

// isRussian: є суто російські літери і немає суто українських.
func isRussian(s string) bool {
	return strings.ContainsAny(s, "ыэёъЫЭЁЪ") && !strings.ContainsAny(s, "іїєґІЇЄҐ")
}

func transliterate(s string) string {
	ru := isRussian(s)
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if t, ok := translitRu[r]; ru && ok {
			b.WriteString(t)
		} else if t, ok := translit[r]; ok {
			b.WriteString(t)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// Службові слова, якими гілка не повинна закінчуватись після обрізання ("...zvitiv-dlya").
var trailingStopWords = map[string]bool{
	"dlya": true, "v": true, "u": true, "na": true, "ta": true, "i": true, "y": true, "z": true,
	"iz": true, "zi": true, "po": true, "do": true, "vid": true, "pro": true, "za": true,
	"pry": true, "pri": true, "cherez": true, "ot": true, "s": true, "k": true, "a": true,
	"for": true, "the": true, "an": true, "of": true, "to": true, "in": true, "on": true,
	"and": true, "with": true, "by": true, "from": true, "at": true, "or": true,
}

// slugify перетворює довільну назву на kebab-case латиницею.
func slugify(s string, maxLen int) string {
	slug := strings.Trim(nonSlug.ReplaceAllString(transliterate(s), "-"), "-")
	if maxLen > 0 && len(slug) > maxLen {
		cut := slug[:maxLen]
		if i := strings.LastIndex(cut, "-"); i > 0 && slug[maxLen] != '-' {
			cut = cut[:i]
		}
		slug = strings.Trim(cut, "-")
	}
	words := strings.Split(slug, "-")
	for len(words) > 1 && trailingStopWords[words[len(words)-1]] {
		words = words[:len(words)-1]
	}
	return strings.Join(words, "-")
}

var nonID = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// normalizeID лишає тільки безпечні для git символи: "#123" -> "123", "PROJ 42" -> "PROJ-42".
func normalizeID(id string) string {
	return strings.Trim(nonID.ReplaceAllString(strings.TrimSpace(id), "-"), "-_")
}

// "PROJ-123 Назва", "[PROJ-123] Назва", "#123: Назва", "123 - Назва"
var leadingID = regexp.MustCompile(`^\s*\[?(#?[A-Za-z][A-Za-z0-9]*-\d+|#?\d+)\]?\s*[:.\-–—|]?\s+(.+)$`)

// extractID дістає ID з початку назви, якщо його вставили разом із заголовком.
func extractID(title string) (id, rest string, ok bool) {
	m := leadingID.FindStringSubmatch(title)
	if m == nil {
		return "", title, false
	}
	return m[1], m[2], true
}

type variant struct {
	Label string
	Value string
}

var typeHints = []struct {
	prefix   string
	keywords []string
}{
	{"hotfix", []string{"hotfix", "urgent", "терміново", "срочно", "критичн", "критическ"}},
	{"bugfix", []string{"bug", "fix", "error", "crash", "broken", "баг", "помилк", "виправ", "не працює", "ошибк", "исправ", "не работает", "падає", "падает"}},
	{"refactor", []string{"refactor", "рефактор", "cleanup"}},
	{"chore", []string{"update dep", "bump", "upgrade", "оновити залеж", "ci", "config"}},
}

// detectType вгадує тип гілки за ключовими словами в назві.
func detectType(title string) string {
	lower := " " + strings.ToLower(title) + " "
	for _, h := range typeHints {
		for _, kw := range h.keywords {
			if containsWord(lower, kw) {
				return h.prefix
			}
		}
	}
	return "feature"
}

// containsWord шукає ключове слово як початок слова (щоб "ci" не матчився в "special").
func containsWord(text, kw string) bool {
	for i := 0; ; {
		j := strings.Index(text[i:], kw)
		if j < 0 {
			return false
		}
		pos := i + j
		if pos == 0 || !isLetter(lastRune(text[:pos])) {
			return true
		}
		i = pos + len(kw)
	}
}

func lastRune(s string) rune {
	r := []rune(s)
	return r[len(r)-1]
}

func isLetter(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// Input — те, що реально пішло в генерацію (після авто-вилучення ID).
type Input struct {
	Title       string
	ID          string
	IDExtracted bool
	Type        string
}

func resolveInput(rawTitle, rawID string) Input {
	in := Input{Title: strings.TrimSpace(rawTitle), ID: normalizeID(rawID)}
	if in.ID == "" {
		if id, rest, ok := extractID(in.Title); ok {
			in.ID, in.Title, in.IDExtracted = normalizeID(id), rest, true
		}
	}
	in.Type = detectType(in.Title)
	return in
}

// Пороги, після яких назва вважається «довгою» і її варто віддати LLM.
const (
	aiMinWords   = 5
	aiMinSlugLen = 31
)

// needsAI: короткі назви локальна транслітерація обробляє добре, довгі — ні.
func needsAI(in Input) bool {
	slug := slugify(in.Title, 0)
	return len(slug) >= aiMinSlugLen || len(strings.Split(slug, "-")) >= aiMinWords
}

// buildAIVariants складає гілки з коротких англійських назв, запропонованих LLM.
func buildAIVariants(in Input, res aiResult) []variant {
	typ := firstNonEmpty(res.Type, in.Type)
	var out []variant
	for _, s := range res.Slugs {
		body := s
		if in.ID != "" {
			body = in.ID + "-" + s
		}
		out = append(out, variant{"✨ AI", typ + "/" + body})
	}
	return out
}

// buildVariants повертає список варіантів, першим — найімовірніший тип.
func buildVariants(in Input) []variant {
	slug := slugify(in.Title, maxSlugLen)
	id := in.ID
	if slug == "" && id == "" {
		return nil
	}

	join := func(parts ...string) string {
		var nonEmpty []string
		for _, p := range parts {
			if p != "" {
				nonEmpty = append(nonEmpty, p)
			}
		}
		return strings.Join(nonEmpty, "-")
	}
	body := join(id, slug)

	prefixes := []string{in.Type}
	for _, p := range []string{"feature", "bugfix", "hotfix", "refactor", "chore"} {
		if p != in.Type {
			prefixes = append(prefixes, p)
		}
	}

	var out []variant
	seen := map[string]bool{}
	add := func(label, value string) {
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		out = append(out, variant{label, value})
	}

	for i, p := range prefixes {
		label := p
		if i == 0 {
			label += " ★"
		}
		add(label, p+"/"+body)
	}
	add("без префікса", body)
	if strings.ToLower(body) != body {
		add("lowercase", in.Type+"/"+strings.ToLower(body))
	}
	if id != "" && slug != "" {
		add("тільки ID", in.Type+"/"+id)
		add("snake_case", in.Type+"/"+id+"_"+strings.ReplaceAll(slug, "-", "_"))
	}
	return out
}
