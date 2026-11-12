package main

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type sel struct {
	text     string
	id       string
	desc     string
	role     string
	contains string
	ref      int
	index    int
}

func (s sel) empty() bool {
	return s.text == "" && s.id == "" && s.desc == "" && s.role == "" && s.contains == "" && s.ref == 0 && s.index == 0
}

func (s sel) String() string {
	var parts []string
	for _, p := range [][2]string{{"text", s.text}, {"id", s.id}, {"desc", s.desc}, {"role", s.role}, {"contains", s.contains}} {
		if p[1] != "" {
			parts = append(parts, fmt.Sprintf("%s=%q", p[0], p[1]))
		}
	}
	if s.ref != 0 {
		parts = append(parts, fmt.Sprintf("ref=%d", s.ref))
	}
	return strings.Join(parts, " ")
}

func (s sel) match(e Element) bool {
	if s.ref != 0 && e.Ref != s.ref {
		return false
	}
	if s.text != "" && !strings.EqualFold(e.Label, s.text) {
		return false
	}
	if s.desc != "" && !strings.EqualFold(e.Desc, s.desc) {
		return false
	}
	if s.id != "" && !strings.Contains(e.ID, s.id) {
		return false
	}
	if s.role != "" && !strings.EqualFold(e.Role, s.role) {
		return false
	}
	if s.contains != "" {
		hay := strings.ToLower(e.Label + " " + e.Desc + " " + e.ID)
		if !strings.Contains(hay, strings.ToLower(s.contains)) {
			return false
		}
	}
	return true
}

var errNoMatch = errors.New("no element matches")

func (s sel) find(sc *Screen) (Element, error) {
	var hits []Element
	for _, e := range sc.Elements {
		if s.match(e) {
			hits = append(hits, e)
		}
	}
	if len(hits) == 0 {
		return Element{}, errNoMatch
	}
	if s.index > 0 {
		if s.index > len(hits) {
			return Element{}, fmt.Errorf("only %d match(es), index %d asked", len(hits), s.index)
		}
		return hits[s.index-1], nil
	}
	if len(hits) > 1 && s.ref == 0 {
		same := true
		for _, h := range hits[1:] {
			if h.Label != hits[0].Label || h.X1 != hits[0].X1 || h.Y1 != hits[0].Y1 {
				same = false
				break
			}
		}
		if !same {
			return Element{}, fmt.Errorf("%d elements match (%s); add --index N or --id", len(hits), describe(hits))
		}
	}
	return hits[0], nil
}

func describe(es []Element) string {
	var parts []string
	for i, e := range es {
		if i == 4 {
			parts = append(parts, "…")
			break
		}
		cx, cy := e.Center()
		parts = append(parts, fmt.Sprintf("[%d] %s %q @%d,%d", e.Ref, e.Role, trunc(e.Label+e.Desc, 24), cx, cy))
	}
	return strings.Join(parts, ", ")
}

func resolve(s sel, all bool) (Element, *Screen) {
	sc := dumpScreen(all)
	e, err := s.find(sc)
	if err != nil {
		if errors.Is(err, errNoMatch) {
			fail("nothing matches %s. Screen:\n%s", s, sc)
		}
		fail("%v", err)
	}
	return e, sc
}

func tap(e Element, longPress bool) {
	cx, cy := e.Center()
	if longPress {
		cmd("input", "swipe", itoa(cx), itoa(cy), itoa(cx), itoa(cy), "800")
		return
	}
	cmd("input", "tap", itoa(cx), itoa(cy))
}

func typeInto(e *Element, text string) {
	if e != nil {
		tap(*e, false)
		time.Sleep(120 * time.Millisecond)
	}
	cmd("input", "text", escapeText(text))
}

// escapeText makes text safe for `input text`: the phone's tool takes %s for
// a space and the device shell still reads &, ; and quotes.
func escapeText(s string) string {
	for _, r := range s {
		if r > unicode.MaxASCII {
			fail("`input text` cannot send %q: it is ASCII only. Use the clipboard path (coming with the companion app) for accents, emoji and non-Latin text.", string(r))
		}
	}
	var b strings.Builder
	const special = `&|;<>()$` + "`" + `\*?'"[]#!~`
	for _, r := range s {
		switch {
		case r == ' ':
			b.WriteString("%s")
		case strings.ContainsRune(special, r):
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

var keyCodes = map[string]int{
	"back": 4, "home": 3, "enter": 66, "tab": 61, "del": 67, "delete": 67,
	"recents": 187, "power": 26, "volup": 24, "voldown": 25, "menu": 82,
	"search": 84, "escape": 111, "space": 62, "up": 19, "down": 20,
	"left": 21, "right": 22,
}

func key(name string) {
	if n, ok := keyCodes[strings.ToLower(name)]; ok {
		cmd("input", "keyevent", strconv.Itoa(n))
		return
	}
	if n, err := strconv.Atoi(name); err == nil {
		cmd("input", "keyevent", strconv.Itoa(n))
		return
	}
	var names []string
	for k := range keyCodes {
		names = append(names, k)
	}
	sort.Strings(names)
	fail("unknown key %q. Known: %s, or a numeric keycode", name, strings.Join(names, " "))
}

func swipe(x1, y1, x2, y2, ms int) {
	cmd("input", "swipe", itoa(x1), itoa(y1), itoa(x2), itoa(y2), itoa(ms))
}

func launch(pkg string) {
	out, err := shell("monkey", "-p", pkg, "-c", "android.intent.category.LAUNCHER", "1")
	if err != nil || strings.Contains(out, "No activities found") || strings.Contains(out, "monkey aborted") {
		fail("cannot launch %s: %s", pkg, strings.TrimSpace(out))
	}
}

func apps() []string {
	out := mustShell("pm", "list", "packages", "-3")
	var ps []string
	for _, line := range strings.Split(out, "\n") {
		if p, ok := strings.CutPrefix(strings.TrimSpace(line), "package:"); ok {
			ps = append(ps, p)
		}
	}
	sort.Strings(ps)
	return ps
}

// state is the cheap read: no uiautomator, one dumpsys, for "which app am I
// in" without paying for a full tree.
func state() string {
	out := mustShell("dumpsys", "window")
	focus, app := "", ""
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if i := strings.Index(line, "mCurrentFocus="); i >= 0 && focus == "" {
			focus = strings.TrimSpace(line[i+len("mCurrentFocus="):])
		}
		if i := strings.Index(line, "mFocusedApp="); i >= 0 && app == "" {
			app = strings.TrimSpace(line[i+len("mFocusedApp="):])
		}
		if focus != "" && app != "" {
			break
		}
	}
	if focus == "" && app == "" {
		return "unknown"
	}
	return strings.TrimSpace(focus + " | " + app)
}

func shot(path string) {
	raw, err := adbOut("exec-out", "screencap", "-p")
	if err != nil || len(raw) < 1000 {
		fail("screencap failed (%v)", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		fail("write %s: %v", path, err)
	}
	fmt.Printf("%s (%d bytes)\n", path, len(raw))
}

func waitFor(s sel, timeout time.Duration) Element {
	deadline := time.Now().Add(timeout)
	for {
		sc := dumpScreen(false)
		if e, err := s.find(sc); err == nil {
			return e
		}
		if time.Now().After(deadline) {
			fail("nothing matched %s within %s. Screen:\n%s", s, timeout, sc)
		}
		time.Sleep(400 * time.Millisecond)
	}
}

func cmd(args ...string) {
	if _, err := shell(args...); err != nil {
		fail("%s: %v", args[0], err)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
