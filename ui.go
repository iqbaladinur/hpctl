package main

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type rawNode struct {
	Text       string    `xml:"text,attr"`
	ID         string    `xml:"resource-id,attr"`
	Class      string    `xml:"class,attr"`
	Desc       string    `xml:"content-desc,attr"`
	Package    string    `xml:"package,attr"`
	Clickable  bool      `xml:"clickable,attr"`
	Enabled    bool      `xml:"enabled,attr"`
	Focused    bool      `xml:"focused,attr"`
	Checked    bool      `xml:"checked,attr"`
	Scrollable bool      `xml:"scrollable,attr"`
	Selected   bool      `xml:"selected,attr"`
	Bounds     string    `xml:"bounds,attr"`
	Nodes      []rawNode `xml:"node"`
}

type Element struct {
	Ref        int
	Role       string
	Label      string
	ID         string
	Desc       string
	X1, Y1     int
	X2, Y2     int
	Clickable  bool
	Enabled    bool
	Focused    bool
	Checked    bool
	Scrollable bool
}

func (e Element) Center() (int, int) { return (e.X1 + e.X2) / 2, (e.Y1 + e.Y2) / 2 }

func (e Element) Empty() bool {
	return e.Label == "" && e.Desc == "" && e.ID == "" && !e.Clickable && !e.Scrollable
}

type Screen struct {
	App      string
	Width    int
	Height   int
	Seq      int64
	Elements []Element
}

var boundsRe = regexp.MustCompile(`\[(-?\d+),(-?\d+)\]\[(-?\d+),(-?\d+)\]`)

// dumpScreen reads the accessibility tree through uiautomator. The tree is
// what the phone itself uses, so an element is named, never guessed from a
// picture.
//
// A dump that caught the accessibility service mid-reconnect can come back
// with the status bar alone; that tree is a few dozen pixels tall, so it is
// retried instead of trusted.
func dumpScreen(all bool) *Screen {
	// The companion's file is the fast path; uiautomator is the fallback and
	// the only one that works without the app installed.
	if !slowMode {
		if s, ok := fastScreen(all); ok {
			return s
		}
	}
	const path = "/data/local/tmp/hpctl.xml"
	var last *Screen
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(700 * time.Millisecond)
		}
		shell("uiautomator", "dump", path)
		raw, err := adbOut("exec-out", "cat", path)
		if err != nil || len(raw) == 0 {
			continue
		}
		s, err := parseScreen(raw, all)
		if err != nil {
			continue
		}
		last = s
		if s.Height > 300 && len(s.Elements) > 0 {
			return s
		}
	}
	if last == nil {
		fail("uiautomator returned nothing (screen off, or a secure window)")
	}
	if len(last.Elements) == 0 {
		fail("uiautomator returned no usable tree (screen may be off or in a secure window)")
	}
	return last
}

func parseScreen(raw []byte, all bool) (*Screen, error) {
	var root rawNode
	if err := xml.Unmarshal(raw, &root); err != nil {
		return nil, err
	}
	s := &Screen{App: root.Package}
	var maxX, maxY int
	seen := map[string]bool{}
	var walk func(n rawNode)
	walk = func(n rawNode) {
		for _, c := range n.Nodes {
			e := Element{
				Role: role(c.Class), Label: strings.TrimSpace(c.Text), ID: c.ID,
				Desc: strings.TrimSpace(c.Desc), Clickable: c.Clickable, Enabled: c.Enabled,
				Focused: c.Focused, Checked: c.Checked, Scrollable: c.Scrollable,
			}
			if m := boundsRe.FindStringSubmatch(c.Bounds); m != nil {
				e.X1, _ = strconv.Atoi(m[1])
				e.Y1, _ = strconv.Atoi(m[2])
				e.X2, _ = strconv.Atoi(m[3])
				e.Y2, _ = strconv.Atoi(m[4])
				if e.X2 > maxX {
					maxX = e.X2
				}
				if e.Y2 > maxY {
					maxY = e.Y2
				}
			}
			visible := e.X2 > e.X1 && e.Y2 > e.Y1
			keep := visible && (all || !e.Empty())
			if keep && len(s.Elements) < 400 {
				key := fmt.Sprintf("%d,%d,%d,%d|%s|%s", e.X1, e.Y1, e.X2, e.Y2, e.Label, e.Role)
				if !seen[key] {
					seen[key] = true
					e.Ref = len(s.Elements) + 1
					s.Elements = append(s.Elements, e)
				}
			}
			walk(c)
		}
	}
	walk(root)
	s.Width, s.Height = maxX, maxY
	if s.App == "" {
		for _, e := range s.Elements {
			if e.ID != "" {
				s.App = strings.SplitN(e.ID, ":", 2)[0]
				break
			}
		}
	}
	return s, nil
}

var roleSuffixes = []string{"Compat", "Layout", "View"}

func role(class string) string {
	if class == "" {
		return "?"
	}
	name := class[strings.LastIndex(class, ".")+1:]
	for _, suf := range roleSuffixes {
		if suf != "View" && strings.HasSuffix(name, suf) {
			name = strings.TrimSuffix(name, suf)
		}
	}
	if strings.EqualFold(name, "View") {
		return "view"
	}
	return strings.ToLower(name)
}

func (s *Screen) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s · %dx%d · %d elemen\n", shortPkg(s.App), s.Width, s.Height, len(s.Elements))
	for _, e := range s.Elements {
		cx, cy := e.Center()
		label := e.Label
		if label == "" {
			label = e.Desc
		}
		fmt.Fprintf(&b, "[%d] %-11s %q @%d,%d", e.Ref, e.Role, trunc(label, 60), cx, cy)
		if e.ID != "" {
			fmt.Fprintf(&b, " id=%s", shortID(e.ID))
		}
		var flags []string
		if e.Focused {
			flags = append(flags, "focused")
		}
		if !e.Enabled {
			flags = append(flags, "disabled")
		}
		if e.Checked {
			flags = append(flags, "checked")
		}
		if e.Scrollable {
			flags = append(flags, "scrollable")
		}
		if len(flags) > 0 {
			b.WriteString(" " + strings.Join(flags, ","))
		}
		if w := riskyLabel(e); w != "" {
			b.WriteString(" ⚠risky:" + w)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func shortPkg(p string) string {
	if p == "" {
		return "?"
	}
	return p
}

func shortID(id string) string {
	if i := strings.Index(id, ":id/"); i >= 0 {
		return id[i+1:]
	}
	return id
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
