package main

import "testing"

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Додати авторизацію через Google": "dodaty-avtoryzatsiyu-cherez-google",
		"Виправити п'ять помилок у формі": "vypravyty-pyat-pomylok-u-formi",
		"Їжак і ґанок":                    "yizhak-i-ganok",
		"Исправить ошибку в API!!!":       "yspravyt-oshybku-v-api", // без ы/э/ё/ъ вважаємо українською
		"Исправить ошибки в выгрузке":     "ispravit-oshibki-v-vygruzke",
		"Сделать выгрузку":                "sdelat-vygruzku",
		"Add  dark mode -- settings":      "add-dark-mode-settings",
		"   ":                             "",
	}
	for in, want := range cases {
		if got := slugify(in, maxSlugLen); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSlugifyTruncatesOnWordBoundary(t *testing.T) {
	got := slugify("one two three four five six seven eight nine ten eleven twelve", 20)
	if got != "one-two-three-four" {
		t.Errorf("got %q", got)
	}
}

func TestSlugifyDropsTrailingStopWords(t *testing.T) {
	got := slugify("Автоматизація формування звітів для бухгалтерії по філіях", maxSlugLen)
	if got != "avtomatyzatsiya-formuvannya-zvitiv" {
		t.Errorf("got %q", got)
	}
}

func TestResolveInputExtractsID(t *testing.T) {
	cases := []struct{ title, id, title2 string }{
		{"PROJ-123 Додати логін", "PROJ-123", "Додати логін"},
		{"[ABC-7] Fix crash", "ABC-7", "Fix crash"},
		{"#42: Нова сторінка", "42", "Нова сторінка"},
		{"Звичайна назва", "", "Звичайна назва"},
	}
	for _, c := range cases {
		in := resolveInput(c.title, "")
		if in.ID != c.id || in.Title != c.title2 {
			t.Errorf("resolveInput(%q) = (%q, %q), want (%q, %q)", c.title, in.ID, in.Title, c.id, c.title2)
		}
	}
	// явно введений ID має пріоритет
	if in := resolveInput("PROJ-1 Title", "#99"); in.ID != "99" || in.Title != "PROJ-1 Title" {
		t.Errorf("explicit ID ignored: %+v", in)
	}
}

func TestDetectType(t *testing.T) {
	cases := map[string]string{
		"Виправити помилку логіну": "bugfix",
		"Fix login crash":            "bugfix",
		"Терміново: впав прод":       "hotfix",
		"Рефакторинг модуля оплат":   "refactor",
		"Додати спеціальну сторінку": "feature",
		"Special offers page":        "feature",
	}
	for in, want := range cases {
		if got := detectType(in); got != want {
			t.Errorf("detectType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildVariants(t *testing.T) {
	v := buildVariants(resolveInput("Виправити помилку в кошику", "SHOP-15"))
	if len(v) == 0 || v[0].Value != "bugfix/SHOP-15-vypravyty-pomylku-v-koshyku" {
		t.Fatalf("unexpected first variant: %+v", v)
	}
	seen := map[string]bool{}
	for _, x := range v {
		if seen[x.Value] {
			t.Errorf("duplicate variant %q", x.Value)
		}
		seen[x.Value] = true
	}
	if buildVariants(resolveInput("", "")) != nil {
		t.Error("expected no variants for empty input")
	}
}
