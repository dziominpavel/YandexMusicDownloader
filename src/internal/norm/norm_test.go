package norm

import "testing"

func TestSortArtistName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Амирчик", "Амирчик"},                // Cyrillic stays
		{"Mari Sa", "Мari Sa"},                // Latin M → Cyrillic М
		{"HEXXENMIND", "НEXXENMIND"},          // Latin H → Cyrillic Н (first char only)
		{"Noize MC", SortPrefix + "Noize MC"}, // N has no twin → prefix
		{"VAVAN", SortPrefix + "VAVAN"},       // V has no twin → prefix
		{"7Б", SortPrefix + "7Б"},             // digit → prefix
		{"JANAGA", SortPrefix + "JANAGA"},     // J has no twin → prefix
		{"", ""},                              // empty stays empty
	}
	for _, c := range cases {
		if got := SortArtistName(c.in); got != c.want {
			t.Errorf("SortArtistName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSortPrefixOrder(t *testing.T) {
	// Byte-wise (codepoint) order must be: Latin < prefix < Cyrillic.
	// SortPrefix itself must be a single invisible rune.
	if SortPrefix != "ι " {
		t.Fatalf("prefix changed: %q", SortPrefix)
	}
	if !("zzz" < SortPrefix+"Noize MC" && SortPrefix+"Noize MC" < "Амирчик") {
		t.Fatal("order must be: latin < prefixed < cyrillic")
	}
}

func TestHasCyrillic(t *testing.T) {
	if !HasCyrillic("Трек") || HasCyrillic("Track") || !HasCyrillic("Ёлка") {
		t.Fatal("cyrillic detection broken")
	}
}

func TestComparable(t *testing.T) {
	cases := []struct{ a, b string }{
		// Разный регистр.
		{"Светлая Полоса", "светлая полоса"},
		// ё == е.
		{"Пёс", "Пес"},
		{"Зелёный", "Зеленый"},
		// Порядок/написание тире не важен.
		{"Мурашки - Взгляд", "Мурашки — Взгляд"},
		{"A - B", "A—B"},
		// Сортировочный префикс iota снимается.
		{SortPrefix + "Noize MC - Светлая полоса", "Noize MC - Светлая полоса"},
		// Латинский двойник как первый символ (правило сортировки).
		{"Мari Sa - Моя Любовь", "Mari Sa - Моя Любовь"},
		{"НEXXENMIND - Камин", "HEXXENMIND - Камин"},
		// Кавычки схлопываются.
		{"«Песня»", `"Песня"`},
	}
	for _, c := range cases {
		if Comparable(c.a) != Comparable(c.b) {
			t.Errorf("Comparable(%q)=%q != Comparable(%q)=%q",
				c.a, Comparable(c.a), c.b, Comparable(c.b))
		}
	}
}

func TestComparableDistinct(t *testing.T) {
	pairs := [][2]string{
		{"Песня", "Песня (Remix)"},
		{"Love", "Hate"},
		{"Альфа", "Бета"},
	}
	for _, p := range pairs {
		if Comparable(p[0]) == Comparable(p[1]) {
			t.Errorf("%q and %q must not collapse to the same key", p[0], p[1])
		}
	}
}

func TestArtistsKeyOrderIndependent(t *testing.T) {
	a := ArtistsKey([]string{"Исполнитель B", "Исполнитель A"})
	b := ArtistsKey([]string{"Исполнитель A", "Исполнитель B"})
	if a == "" || a != b {
		t.Fatalf("order must not matter: %q vs %q", a, b)
	}
	// Отсортированные токены: результат детерминирован.
	if want := ArtistsKey([]string{"Исполнитель A", "Исполнитель B"}); a != want {
		t.Fatalf("got %q want %q", a, want)
	}
	// Пустые имена отбрасываются.
	if ArtistsKey([]string{"", "  ", "Noize MC"}) != ArtistsKey([]string{"Noize MC"}) {
		t.Fatal("blank artists must be dropped")
	}
	// Имя со склеенными через запятую исполнителями (файл/тег) эквивалентно
	// списку из нескольких имён.
	joined := ArtistsKey([]string{"Исполнитель B, Исполнитель A"})
	split := ArtistsKey([]string{"Исполнитель A", "Исполнитель B"})
	if joined == "" || joined != split {
		t.Fatalf("comma-joined names must split: %q vs %q", joined, split)
	}
}

func TestStripTrackNumber(t *testing.T) {
	cases := []struct{ in, want string }{
		{"01 - Песня", "Песня"},
		{"01. Песня", "Песня"},
		{"1) Песня", "Песня"},
		{"01", ""},
		{"2Pac - Hit", "2Pac - Hit"},         // не номер трека
		{"2024 - Сборник", "2024 - Сборник"}, // год не трогаем
		{"Песня", "Песня"},
	}
	for _, c := range cases {
		if got := StripTrackNumber(c.in); got != c.want {
			t.Errorf("StripTrackNumber(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
