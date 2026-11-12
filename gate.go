package main

import (
	"fmt"
	"os"
	"strings"
)

// A gate, not a sandbox. hpctl cannot tell whether the person really said yes,
// because the agent and this binary share the machine. What it can do is make
// an irreversible tap impossible to fire by accident: the command stops, says
// what it was about to do, and needs --yes to go on.
var riskWords = []string{
	"kirim", "send", "bayar", "pay", "beli", "buy", "checkout", "order",
	"transfer", "hapus", "delete", "uninstall", "install", "pasang",
	"top up", "topup", "withdraw", "tarik", "salin", "post", "unggah", "upload",
	"publish", "kirim sekarang", "pesan", "call", "telepon",
}

var riskPkgs = []string{
	"com.bca", "com.bni", "com.bri", "id.co.bankmandiri", "com.btpn",
	"com.dana", "com.ovo", "com.gojek", "com.gopay", "id.dana",
	"com.shopee", "com.tokopedia", "com.lazada", "com.bukalapak",
	"com.android.vending", "com.google.android.apps.walletnfcrel",
	"com.bitfinex", "com.binance", "com.coinbase",
}

func loadRiskPkgs() []string {
	out := append([]string{}, riskPkgs...)
	path := os.Getenv("HOME") + "/.config/hpctl/risky-apps.txt"
	if b, err := os.ReadFile(path); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				out = append(out, line)
			}
		}
	}
	return out
}

func riskyLabel(e Element) string {
	// A text field echoing what was typed is not an action; only an element
	// that can be pressed counts.
	if e.Role == "edittext" {
		return ""
	}
	switch e.Role {
	case "text", "textview":
		if !e.Clickable {
			return ""
		}
	}
	hay := strings.ToLower(e.Label + " " + e.Desc)
	for _, w := range riskWords {
		if strings.Contains(hay, w) {
			return w
		}
	}
	return ""
}

func riskyPkg(pkg string) string {
	for _, p := range loadRiskPkgs() {
		if pkg == p || strings.HasPrefix(pkg, p+".") {
			return p
		}
	}
	return ""
}

// gate stops a risky action unless --yes was passed. It prints the exact call
// to repeat, so the retry carries no new decision.
func gate(yes bool, what, why string) {
	if yes {
		return
	}
	fmt.Fprintf(os.Stderr, "NEEDS_APPROVAL: %s\n  alasan: %s\n  Ask the person in the chat first. Only after an explicit yes, repeat the same command with --yes.\n", what, why)
	os.Exit(3)
}
