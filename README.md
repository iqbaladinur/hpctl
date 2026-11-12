# hpctl

Drive an Android phone from the command line **by element name, not by pixel**.

`hpctl` reads the phone's own accessibility tree and acts on what it finds:
`hpctl tap --text "Send"` instead of `input tap 981 2104`. Coordinates are
resolved fresh every time, so a layout change or a different screen size does
not break your script.

```
$ hpctl obs
com.zhiliaoapp.musically · 1080x2160 · 34 elemen
[1] viewgroup  "" @540,1092
[2] text       "For You" @270,120
[3] button     "Like" @1002,1855 id=id/like
...
$ hpctl tap --text "Search"
```

## Why

| | `uiautomator dump` | hpctl + companion |
|---|---|---|
| read the screen | 1.7–1.8 s | 0.04–0.09 s |
| tap + verify | ~1.8 s | ~1.2 s |
| app in focus | 0.5 s | 0.13 s |

The companion app (optional, `android/`) keeps a JSON copy of the current screen
on disk, refreshed on every UI event plus a 900 ms tick. Reading it is one
`adb exec-out cat`. Without the companion, `hpctl` falls back to `uiautomator`
automatically — nothing breaks, it is just slower.

## Requirements

- Linux/macOS with `adb` (Android platform-tools)
- Go 1.27+ to build the CLI
- An Android device with USB debugging enabled (`adb devices` must list it)
- Optional: JDK 17 + Android SDK 35 to build the companion app

## Install

```bash
git clone https://github.com/iqbaladinur/hpctl
cd hpctl
go build -o hpctl .
install -m755 hpctl ~/.local/bin/hpctl
```

Zero Go dependencies — the module imports only the standard library.

## Commands

```
hpctl devices                        terpasang apa
hpctl obs [--all]                    baca layar: [ref] role "label" @x,y
hpctl state                          app aktif, murah (tanpa dump)
hpctl tap --text "Kirim"             tap by exact label
hpctl tap --contains "kirim"         tap by substring
hpctl tap --id send_button           tap by resource-id
hpctl tap -r 12                      tap by ref from obs
hpctl tap --text "Post" --index 2    the 2nd match when labels repeat
hpctl tap --long --text "Item"       long press
hpctl type "hello" --into "Message"  write into a field
hpctl type --clear --into "Search"   empty a field
hpctl key back|home|enter|recents    or a numeric keycode
hpctl swipe 540 1800 540 400 300     x1 y1 x2 y2 [ms]
hpctl launch com.pkg                 start an app
hpctl wait --text "Home" --timeout 10
hpctl shot screen.png                screenshot
```

Every action prints the resulting screen — there is no need to call `obs`
again. Use `--quiet` to suppress it.

**Selectors:** `--text` (exact, case-insensitive), `--contains` (substring of
label/desc/id), `--id`, `--desc`, `--role`, `-r`/`--ref N` (number from the last
`obs`), `--index N` when several elements match.

**Global flags:** `--device SERIAL`, `--slow` (always use `uiautomator`, skip the
companion), `--quiet`, `--settle MS`. `HPCTL_DEVICE` sets the default serial; with
more than one phone attached `hpctl` refuses rather than guessing.

## Risk gate

`tap`, `type` and `launch` stop with exit code `3` and a `NEEDS_APPROVAL:` line
when the target looks irreversible or financial — labels containing *kirim /
send / bayar / pay / beli / buy / transfer / hapus / delete / post / upload /
call*, or app packages like `com.bca`, `id.dana`, `com.shopee`,
`com.android.vending`. Re-run with `--yes` to proceed. Extra packages can be
listed one per line in `~/.config/hpctl/risky-apps.txt`.

This is a seatbelt, not a sandbox: an agent running on the same machine can pass
`--yes` itself. Real human-in-the-loop needs an approver outside the agent's
path.

## Companion app (optional, faster)

An `AccessibilityService` that writes the screen tree to
`/sdcard/Android/data/id.iqbaldev.hpctl.companion/files/tree.json` on every UI
event (debounced 120 ms) and on a 900 ms tick. `hpctl` reads that file when it
is fresh (< 4 s) and falls back to `uiautomator` when it is not, so a stopped
service never yields stale data.

```bash
cd android
# JDK 17 + Android SDK 35 required
./gradlew :app:assembleDebug
adb install -r app/build/outputs/apk/debug/app-debug.apk
adb shell settings put secure enabled_accessibility_services id.iqbaldev.hpctl.companion/.TreeDumpService
adb shell settings put secure accessibility_enabled 1
```

Turn it off again with:

```bash
adb shell settings put secure enabled_accessibility_services ""
```

The service also handles `hpctl type` while it is running: text is written with
`ACTION_SET_TEXT`, so any script works (emoji, Cyrillic, Arabic — no keyboard,
clipboard untouched). Without it, `type` falls back to `input text`, which is
ASCII only.

**Clock skew:** the phone's clock is not the computer's. `hpctl` measures the
offset once and caches it in `~/.cache/hpctl/clock.json` (valid 5 minutes).
Delete that file if `obs` suddenly always takes the slow path.

## Limits worth knowing

- **Apps that render their own surface have no tree.** Games, emulators and
  video players expose a single full-screen node; the companion cannot help
  there either. Use app intents, or coordinates as a last resort.
- **Secure windows** (lock screen, some banking apps) refuse `uiautomator`
  dumps entirely.
- **Screen off = empty dump.** `adb shell svc power stayon usb` or
  `hpctl key 224` first.
- A third-party **accessibility service already running** can swallow the dump
  (the symptom: `obs` returns only 13 system-UI nodes). Disable the other
  service, or use the companion app.

## Privacy

The companion writes whatever is on screen — including messages and passwords —
to a file that `adb shell` can read. It only writes while the service is
enabled, and the file lives in the app's own external files dir (removed on
uninstall). Its text-typing receiver is exported so `adb shell` can call it,
which means any app on the phone could too. Enable it only on a device you
control, and turn the service off when you are done.

## License

MIT — see [LICENSE](LICENSE). **Use at your own risk:** this tool injects input
events into a real device and can tap irreversible buttons. The risk gate is a
convenience, not a guarantee.
