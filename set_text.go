package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const setTextAction = "id.iqbaldev.hpctl.SET_TEXT"

type setResult struct {
	Req   string `json:"req"`
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

// setTextViaCompanion asks the accessibility service to write the text with
// ACTION_SET_TEXT. Unlike `input text` it takes any script and leaves the
// keyboard and clipboard alone. The service answers through a file, because a
// broadcast cannot return a value.
func setTextViaCompanion(text, into string, appendMode bool) (bool, string) {
	req := fmt.Sprintf("%d", time.Now().UnixNano())
	line := "am broadcast -a " + setTextAction + " -p " + companionPkg +
		" --es req " + shellQuote(req)
	if text == "" {
		line += " --ez clear true"
	} else {
		line += " --es text " + shellQuote(text)
	}
	if into != "" {
		line += " --es into " + shellQuote(into)
	}
	if appendMode {
		line += " --ez append true"
	}
	if _, err := adbOut("shell", line); err != nil {
		return false, err.Error()
	}
	deadline := time.Now().Add(2500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if r, ok := readSetResult(); ok && r.Req == req {
			return r.OK, r.Error
		}
		time.Sleep(70 * time.Millisecond)
	}
	return false, "companion did not answer"
}

func readSetResult() (setResult, bool) {
	for _, p := range companionPaths {
		raw, err := adbOut("exec-out", "cat", strings.Replace(p, "tree.json", "set_result.json", 1))
		if err != nil || len(raw) < 8 {
			continue
		}
		var r setResult
		if json.Unmarshal(raw, &r) == nil {
			return r, true
		}
	}
	return setResult{}, false
}

// shellQuote makes one argument survive adb's shell join on the device.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func isASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}

// verifyText re-reads the screen until the field shows what was written. The
// companion can answer "ok" before the app re-renders, so a silent success is
// not proof.
func verifyText(into, want string, within time.Duration) bool {
	deadline := time.Now().Add(within)
	for {
		if sc, ok := fastScreen(false); ok {
			for _, e := range sc.Elements {
				if e.Role != "edittext" && e.Role != "edittextview" && !strings.Contains(strings.ToLower(e.Role), "edit") {
					continue
				}
				match := into == "" || strings.Contains(strings.ToLower(e.ID+" "+e.Label), strings.ToLower(into))
				if !match {
					continue
				}
				if e.Label == want {
					return true
				}
			}
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(120 * time.Millisecond)
	}
}
