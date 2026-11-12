package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var (
	device   string
	slowMode bool // --slow: always use uiautomator, never the companion file
)

func adbArgs(args ...string) []string {
	if device != "" {
		return append([]string{"-s", device}, args...)
	}
	return args
}

func adbOut(args ...string) ([]byte, error) {
	cmd := exec.Command("adb", adbArgs(args...)...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	if err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = strings.TrimSpace(out.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return out.Bytes(), fmt.Errorf("%s", msg)
	}
	return out.Bytes(), nil
}

func shell(args ...string) (string, error) {
	out, err := adbOut(append([]string{"shell"}, args...)...)
	return string(out), err
}

func mustShell(args ...string) string {
	out, err := shell(args...)
	if err != nil {
		fail("%v", err)
	}
	return out
}

// devices lists the attached serials, so a missing or ambiguous phone is
// reported instead of adb picking one on its own.
func devices() []string {
	out, err := adbOut("devices")
	if err != nil {
		return nil
	}
	var ds []string
	for _, line := range strings.Split(string(out), "\n")[1:] {
		f := strings.Fields(line)
		if len(f) >= 2 && f[1] == "device" {
			ds = append(ds, f[0])
		}
	}
	return ds
}

func pickDevice() {
	if device != "" {
		return
	}
	if env := os.Getenv("HPCTL_DEVICE"); env != "" {
		device = env
		return
	}
	ds := devices()
	switch len(ds) {
	case 0:
		fail("no phone attached. Plug it in (USB debugging on) or `adb connect host:port`.")
	case 1:
		device = ds[0]
	default:
		fail("%d phones attached (%s). Pass --device SERIAL or set HPCTL_DEVICE.", len(ds), strings.Join(ds, ", "))
	}
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "hpctl: "+format+"\n", a...)
	os.Exit(1)
}
