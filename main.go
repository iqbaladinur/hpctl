package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

const usage = `hpctl — kendalikan HP Android lewat adb, tanpa menyimpan koordinat.

  hpctl obs [--all]                    baca layar: [ref] role "label" @x,y
  hpctl tap --text "Kirim"             tap elemen berdasarkan nama
  hpctl tap -r 12                      tap berdasarkan ref dari obs terakhir
  hpctl type "halo" --into "Pesan"     ketik ke field
  hpctl key back|home|enter|N          tekan tombol
  hpctl swipe X1 Y1 X2 Y2 [ms]         geser (default 300ms)
  hpctl launch com.pkg                 buka app
  hpctl apps                           daftar app terpasang
  hpctl state                          app aktif (murah, tanpa dump)
  hpctl wait --text "Beranda"          tunggu sampai elemen muncul
  hpctl shot [file.png]                screenshot
  hpctl devices                        HP yang terpasang
  hpctl version                        versi build

Selector: --text (exact), --contains (substring), --id, --desc, --role,
--index N (yang ke-N kalau banyak), -r/--ref N (nomor dari obs).

Flag umum: --device SERIAL, --quiet (jangan cetak layar setelah aksi),
--settle MS (jeda sebelum baca layar, default 250).
`

// version is stamped at build time: -ldflags "-X main.version=1.2.3".
var version = "dev"

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Print(usage)
		return
	}
	args = stripDevice(args)
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Print(usage)
		return
	}
	// Before pickDevice: `hpctl version` must work with no phone plugged in.
	if args[0] == "version" || args[0] == "-v" || args[0] == "--version" {
		fmt.Println("hpctl " + version)
		return
	}
	pickDevice()

	switch args[0] {
	case "devices":
		fmt.Println(strings.Join(devices(), "\n"))
	case "obs", "observe":
		f, s := newFS("obs", "baca layar")
		all := f.Bool("all", false, "tampilkan juga elemen tanpa label")
		quiet := f.Bool("quiet", false, "baca saja, jangan cetak layar")
		parse(f, args[1:])
		_ = s
		if *quiet {
			dumpScreen(*all)
			return
		}
		fmt.Print(dumpScreen(*all))
	case "tap":
		f, s := newFS("tap", "tap elemen")
		addSel(f, s)
		long := f.Bool("long", false, "tekan lama")
		yes := f.Bool("yes", false, "lanjut walau berisiko (hanya setelah user setuju)")
		q, settle := actionFlags(f)
		parse(f, args[1:])
		requireSel(*s, "tap")
		e, sc := resolve(*s, false)
		label := e.Label + e.Desc
		if w := riskyLabel(e); w != "" {
			gate(*yes, fmt.Sprintf("tap %q di %s", trunc(label, 40), sc.App), fmt.Sprintf("label mengandung %q", w))
		}
		if p := riskyPkg(sc.App); p != "" {
			gate(*yes, fmt.Sprintf("tap %q di %s", trunc(label, 40), sc.App), "app keuangan/pembayaran: "+p)
		}
		seq := sc.Seq
		tap(e, *long)
		after(*q, *settle, e, seq)
	case "type":
		f, s := newFS("type", "ketik teks")
		into := f.String("into", "", "label field tujuan (default: field yang fokus)")
		addSel(f, s)
		yes := f.Bool("yes", false, "lanjut walau berisiko")
		clear := f.Bool("clear", false, "kosongkan field (teks kosong)")
		q, _ := actionFlags(f)
		rest := parse(f, args[1:])
		text := strings.Join(rest, " ")
		if text == "" && !*clear {
			fail("type needs text: hpctl type \"halo\" --into \"Pesan\"  (atau --clear)")
		}
		if p := riskyPkg(state()); p != "" {
			gate(*yes, fmt.Sprintf("type %q di %s", trunc(text, 30), p), "app keuangan/pembayaran: "+p)
		}
		if !slowMode && companionAlive() {
			intoLabel := *into
			if intoLabel == "" {
				intoLabel = s.contains
			}
			var seq int64
			if sc, ok := fastScreen(false); ok {
				seq = sc.Seq
			}
			if ok, msg := setTextViaCompanion(text, intoLabel, false); ok {
				if !verifyText(intoLabel, text, 2*time.Second) {
					fmt.Fprintln(os.Stderr, "hpctl: set_text dilaporkan sukses tapi field belum berubah — cek layarnya di bawah")
				}
				after(*q, 0, Element{}, seq)
				return
			} else if !isASCII(text) {
				fail("companion tidak bisa mengisi field (%s), dan `input text` tidak bisa teks non-ASCII", msg)
			}
		}
		var target *Element
		if *into != "" {
			s.text = *into
		}
		if !s.empty() {
			e, _ := resolve(*s, false)
			target = &e
		}
		typeInto(target, text)
		after(*q, 250, Element{}, 0)
	case "key":
		f, _ := newFS("key", "tekan tombol")
		q, settle := actionFlags(f)
		rest := parse(f, args[1:])
		if len(rest) == 0 {
			fail("key needs a name: hpctl key back")
		}
		key(rest[0])
		after(*q, *settle, Element{}, 0)
	case "swipe":
		f, _ := newFS("swipe", "geser")
		q, settle := actionFlags(f)
		rest := parse(f, args[1:])
		if len(rest) < 4 {
			fail("swipe needs X1 Y1 X2 Y2 [ms]")
		}
		var n [5]int
		for i := 0; i < 4; i++ {
			n[i] = atoi(rest[i])
		}
		n[4] = 300
		if len(rest) > 4 {
			n[4] = atoi(rest[4])
		}
		swipe(n[0], n[1], n[2], n[3], n[4])
		after(*q, *settle, Element{}, 0)
	case "launch":
		f, _ := newFS("launch", "buka app")
		yes := f.Bool("yes", false, "lanjut walau berisiko")
		q, settle := actionFlags(f)
		rest := parse(f, args[1:])
		if len(rest) == 0 {
			fail("launch needs a package: hpctl launch com.zhiliaoapp.musically")
		}
		pkg := rest[0]
		if p := riskyPkg(pkg); p != "" {
			gate(*yes, "launch "+pkg, "app keuangan/pembayaran: "+p)
		}
		launch(pkg)
		after(*q, *settle, Element{}, 0)
	case "apps":
		for _, p := range apps() {
			fmt.Println(p)
		}
	case "state":
		fmt.Println(state())
	case "wait":
		f, s := newFS("wait", "tunggu elemen")
		addSel(f, s)
		to := f.Duration("timeout", 15*time.Second, "batas tunggu")
		parse(f, args[1:])
		requireSel(*s, "wait")
		e := waitFor(*s, *to)
		cx, cy := e.Center()
		fmt.Printf("found [%d] %s %q @%d,%d\n", e.Ref, e.Role, trunc(e.Label+e.Desc, 50), cx, cy)
	case "shot":
		f, _ := newFS("shot", "screenshot")
		out := f.String("o", "", "path keluaran")
		rest := parse(f, args[1:])
		p := *out
		if p == "" && len(rest) > 0 {
			p = rest[0]
		}
		if p == "" {
			p = fmt.Sprintf("screen-%s.png", time.Now().Format("150405"))
		}
		shot(p)
	default:
		fail("unknown command %q. Run hpctl -h.", args[0])
	}
}

func newFS(name, about string) (*flag.FlagSet, *sel) {
	f := flag.NewFlagSet(name, flag.ExitOnError)
	f.Usage = func() { fmt.Print(usage) }
	return f, &sel{}
}

func addSel(f *flag.FlagSet, s *sel) {
	f.StringVar(&s.text, "text", "", "label persis")
	f.StringVar(&s.contains, "contains", "", "potongan label/id/desc")
	f.StringVar(&s.id, "id", "", "potongan resource-id")
	f.StringVar(&s.desc, "desc", "", "content-desc persis")
	f.StringVar(&s.role, "role", "", "role: button, edittext, text, image")
	f.IntVar(&s.index, "index", 0, "yang ke-N kalau banyak")
	f.IntVar(&s.ref, "r", 0, "ref dari obs")
	f.IntVar(&s.ref, "ref", 0, "ref dari obs")
}

func actionFlags(f *flag.FlagSet) (*bool, *int) {
	q := f.Bool("quiet", false, "jangan cetak layar sesudah aksi")
	settle := f.Int("settle", 250, "jeda ms sebelum baca layar")
	return q, settle
}

// parse accepts flags anywhere, not only before the positional arguments, and
// returns the positionals. `hpctl launch com.dana --yes` has to work: a gate
// that needs its bypass written before the package is a trap.
func parse(f *flag.FlagSet, args []string) []string {
	var flags, rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			rest = append(rest, args[i+1:]...)
			break
		}
		if len(a) > 1 && a[0] == '-' {
			name := strings.TrimLeft(a, "-")
			_, _, hasVal := strings.Cut(name, "=")
			if !hasVal && !isBoolFlag(f, name) && i+1 < len(args) {
				flags = append(flags, a, args[i+1])
				i++
				continue
			}
			flags = append(flags, a)
			continue
		}
		rest = append(rest, a)
	}
	if err := f.Parse(flags); err != nil {
		os.Exit(2)
	}
	return rest
}

type boolFlag interface{ IsBoolFlag() bool }

func isBoolFlag(f *flag.FlagSet, name string) bool {
	if fl := f.Lookup(name); fl != nil {
		if b, ok := fl.Value.(boolFlag); ok {
			return b.IsBoolFlag()
		}
	}
	return false
}

func requireSel(s sel, name string) {
	if s.empty() {
		fail("%s needs a selector: --text \"Kirim\", -r 12, --id send, --contains kirim", name)
	}
}

func after(quiet bool, settleMS int, tapped Element, prevSeq int64) {
	if quiet {
		return
	}
	var sc *Screen
	if !slowMode && prevSeq != 0 {
		// After an action the companion needs a moment to report the new
		// screen; waiting for its sequence number beats a fixed sleep.
		sc, _ = fastWaitChange(prevSeq, 3500*time.Millisecond)
	}
	if settleMS > 0 && sc == nil {
		time.Sleep(time.Duration(settleMS) * time.Millisecond)
	}
	if sc == nil {
		sc = dumpScreen(false)
	}
	fmt.Print(sc)
	if tapped.Ref != 0 {
		cx, cy := tapped.Center()
		fmt.Printf("(tapped [%d] @%d,%d)\n", tapped.Ref, cx, cy)
	}
}

func stripDevice(args []string) []string {
	out := args[:0]
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--device" && i+1 < len(args):
			device = args[i+1]
			i++
		case strings.HasPrefix(a, "--device="):
			device = strings.TrimPrefix(a, "--device=")
		case a == "--slow":
			slowMode = true
		default:
			out = append(out, a)
		}
	}
	return out
}

func atoi(s string) int {
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		fail("%q is not a number", s)
	}
	return n
}
