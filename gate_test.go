package main

import "testing"

func TestRiskyLabel(t *testing.T) {
	cases := []struct {
		label string
		want  bool
	}{
		{"Kirim", true},
		{"Send", true},
		{"Post", true},
		{"Bayar sekarang", true},
		{"Hapus", true},
		{"Call", true},
		{"Kirim pesan", true},
		{"Network & internet", false},
		{"Wi-Fi", false},
		{"Stopwatch", false},
		{"", false},
	}
	for _, c := range cases {
		got := riskyLabel(Element{Label: c.label}) != ""
		if got != c.want {
			t.Errorf("riskyLabel(%q) = %v, want %v", c.label, got, c.want)
		}
	}
}

func TestRiskyPkg(t *testing.T) {
	cases := []struct {
		pkg  string
		want bool
	}{
		{"com.dana", true},
		{"id.dana", true},
		{"com.bca", true},
		{"com.shopee", true},
		{"com.android.settings", false},
		{"com.best.deskclock", false},
	}
	for _, c := range cases {
		got := riskyPkg(c.pkg) != ""
		if got != c.want {
			t.Errorf("riskyPkg(%q) = %v, want %v", c.pkg, got, c.want)
		}
	}
}

func TestEscapeText(t *testing.T) {
	if got := escapeText("halo dunia"); got != "halo%sdunia" {
		t.Errorf("space not escaped: %q", got)
	}
	if got := escapeText("a&b"); got != `a\&b` {
		t.Errorf("ampersand not escaped: %q", got)
	}
}

func TestParseFlagsAnywhere(t *testing.T) {
	f, _ := newFS("launch", "")
	yes := f.Bool("yes", false, "")
	rest := parse(f, []string{"com.dana", "--yes"})
	if len(rest) != 1 || rest[0] != "com.dana" {
		t.Fatalf("positional lost: %v", rest)
	}
	if !*yes {
		t.Fatal("--yes after the positional was not parsed")
	}
}
