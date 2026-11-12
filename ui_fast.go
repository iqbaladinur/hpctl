package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The companion app keeps the current screen's tree on disk, so reading it is
// one `cat` instead of a 1.7 s uiautomator run. Everything here is a cache:
// a stale or missing file is not an error, it just means falling back.
const companionPkg = "id.iqbaldev.hpctl.companion"

var companionPaths = []string{
	"/sdcard/Android/data/" + companionPkg + "/files/tree.json",
	"/storage/emulated/0/Android/data/" + companionPkg + "/files/tree.json",
}

const fastMaxAge = 4 * time.Second

type fastTree struct {
	Seq   int64      `json:"seq"`
	Ts    int64      `json:"ts"`
	Pkg   string     `json:"pkg"`
	W     int        `json:"w"`
	H     int        `json:"h"`
	Nodes []fastNode `json:"nodes"`
}

type fastNode struct {
	C   string `json:"c"`
	T   string `json:"t"`
	D   string `json:"d"`
	I   string `json:"i"`
	B   [4]int `json:"b"`
	Cl  int    `json:"cl"`
	Dis int    `json:"dis"`
	F   int    `json:"f"`
	Sc  int    `json:"sc"`
	Ch  int    `json:"ch"`
}

func fastAvailable() bool {
	_, ok := readFast()
	return ok
}

func readFast() (*fastTree, bool) {
	for _, p := range companionPaths {
		raw, err := adbOut("exec-out", "cat", p)
		if err != nil || len(raw) < 16 {
			continue
		}
		var t fastTree
		if json.Unmarshal(raw, &t) != nil {
			continue
		}
		age := time.Since(time.UnixMilli(t.Ts + offsetMillis()))
		if age > fastMaxAge || age < -2*time.Minute {
			continue
		}
		return &t, true
	}
	return nil, false
}

func fastScreen(all bool) (*Screen, bool) {
	t, ok := readFast()
	if !ok {
		return nil, false
	}
	s := &Screen{App: t.Pkg, Width: t.W, Height: t.H, Seq: t.Seq}
	seen := map[string]bool{}
	for _, n := range t.Nodes {
		e := Element{
			Role:       roleFromSimple(n.C),
			Label:      strings.TrimSpace(n.T),
			Desc:       strings.TrimSpace(n.D),
			ID:         n.I,
			X1:         n.B[0],
			Y1:         n.B[1],
			X2:         n.B[2],
			Y2:         n.B[3],
			Clickable:  n.Cl == 1,
			Enabled:    n.Dis != 1,
			Focused:    n.F == 1,
			Scrollable: n.Sc == 1,
			Checked:    n.Ch == 1,
		}
		if e.X2 <= e.X1 || e.Y2 <= e.Y1 {
			continue
		}
		if !all && e.Empty() {
			continue
		}
		key := fmt.Sprintf("%d,%d,%d,%d|%s|%s", e.X1, e.Y1, e.X2, e.Y2, e.Label, e.Role)
		if seen[key] {
			continue
		}
		seen[key] = true
		e.Ref = len(s.Elements) + 1
		s.Elements = append(s.Elements, e)
	}
	if len(s.Elements) == 0 {
		return nil, false
	}
	return s, true
}

// dump waits until the companion reports a change, so the screen printed after
// an action is the one the action produced, not the one before it.
func fastWaitChange(prevSeq int64, timeout time.Duration) (*Screen, bool) {
	deadline := time.Now().Add(timeout)
	for {
		if s, ok := fastScreen(false); ok && s.Seq != prevSeq {
			return s, true
		}
		if time.Now().After(deadline) {
			return fastScreen(false)
		}
		time.Sleep(80 * time.Millisecond)
	}
}

func roleFromSimple(c string) string {
	if c == "" {
		return "?"
	}
	c = strings.TrimSuffix(c, "Compat")
	for _, suf := range []string{"Layout"} {
		if strings.HasSuffix(c, suf) && len(c) > len(suf) {
			c = strings.TrimSuffix(c, suf)
		}
	}
	return strings.ToLower(c)
}

// offsetMillis is host clock minus phone clock. The two are not the same: on
// this phone the difference was 8 minutes, which made every fresh dump look
// hours old. It is measured once and cached, since a phone's clock drifts
// slowly.
func offsetMillis() int64 {
	path := filepath.Join(os.Getenv("HOME"), ".cache", "hpctl", "clock.json")
	type cached struct {
		Offset int64 `json:"offset"`
		At     int64 `json:"at"`
	}
	if raw, err := os.ReadFile(path); err == nil {
		var c cached
		if json.Unmarshal(raw, &c) == nil && time.Since(time.UnixMilli(c.At)) < 5*time.Minute {
			return c.Offset
		}
	}
	out, err := adbOut("shell", "date", "+%s%3N")
	if err != nil {
		return 0
	}
	dev, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil || dev == 0 {
		return 0
	}
	now := time.Now().UnixMilli()
	c := cached{Offset: now - dev, At: now}
	if b, err := json.Marshal(c); err == nil {
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, b, 0o644)
	}
	return c.Offset
}

// companionAlive reports whether the service has written recently, so `type`
// does not spend 2.5 s waiting for an answer that cannot come.
func companionAlive() bool {
	t, ok := readFast()
	return ok && t != nil
}
