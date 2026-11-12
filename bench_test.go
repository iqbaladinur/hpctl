package main

import (
	"encoding/json"
	"os"
	"testing"
)

// Fixtures are real-shaped dumps, checked in so the numbers are reproducible
// without a device: testdata/screen.json is a feed screen (77 nodes),
// screen-large.json a long settings list (261 nodes), screen.xml the same
// feed as uiautomator returns it.
func loadTree(tb testing.TB, path string) *fastTree {
	tb.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		tb.Fatalf("read %s: %v", path, err)
	}
	var t fastTree
	if err := json.Unmarshal(raw, &t); err != nil {
		tb.Fatalf("decode %s: %v", path, err)
	}
	return &t
}

func loadXML(tb testing.TB, path string) []byte {
	tb.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		tb.Fatalf("read %s: %v", path, err)
	}
	return raw
}

// BenchmarkCachedLookup is the headline number: from the raw bytes on disk to
// the element you asked for, i.e. what a lookup costs once the companion has
// written tree.json.
func BenchmarkCachedLookup(b *testing.B) {
	for _, tc := range []struct{ name, path, want string }{
		{"feed-77", "testdata/screen.json", "For You"},
		{"settings-261", "testdata/screen-large.json", "Setting item 40"},
	} {
		raw, err := os.ReadFile(tc.path)
		if err != nil {
			b.Fatalf("read %s: %v", tc.path, err)
		}
		s := sel{text: tc.want}
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				var t fastTree
				if err := json.Unmarshal(raw, &t); err != nil {
					b.Fatal(err)
				}
				sc := screenFromFast(&t, false)
				if sc == nil {
					b.Fatal("empty screen")
				}
				if _, err := s.find(sc); err != nil {
					b.Fatalf("no match: %v", err)
				}
			}
		})
	}
}

func BenchmarkDecodeCompanionTree(b *testing.B) {
	for _, path := range []string{"testdata/screen.json", "testdata/screen-large.json"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			b.Fatalf("read %s: %v", path, err)
		}
		b.Run(path, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				var t fastTree
				if err := json.Unmarshal(raw, &t); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkParseUIAutomatorXML times the fallback path: decoding the XML dump
// the phone produces when the companion is not installed.
func BenchmarkParseUIAutomatorXML(b *testing.B) {
	raw := loadXML(b, "testdata/screen.xml")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := parseScreen(raw, false); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkResolveByID(b *testing.B) {
	t := loadTree(b, "testdata/screen-large.json")
	// 52 rows share the switch id; picking one is the realistic case.
	s := sel{id: "switch_widget", index: 40}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		sc := screenFromFast(t, false)
		if _, err := s.find(sc); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRenderScreen times what the agent actually reads: the text form of
// the screen printed after every action.
func BenchmarkRenderScreen(b *testing.B) {
	for _, path := range []string{"testdata/screen.json", "testdata/screen-large.json"} {
		t := loadTree(b, path)
		sc := screenFromFast(t, false)
		b.Run(path, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if out := sc.String(); len(out) == 0 {
					b.Fatal("empty render")
				}
			}
		})
	}
}
