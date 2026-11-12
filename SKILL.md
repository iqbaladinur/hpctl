---
name: hpctl
description: Use when driving a physical Android phone from the shell (read the screen, tap, type, swipe) — element-name automation over adb with no stored coordinates.
---

# hpctl — drive an Android phone by name

`hpctl` talks to a device over `adb`, reads the phone's own accessibility tree,
and acts on an element by label or resource-id. You never write pixel
coordinates, so the same command survives layout changes and other screen sizes.

Use it for: posting, replying, navigating an app, filling a form, checking what
is on screen, scripting a repetitive tap sequence.
Do **not** use it for: games, emulators or video players that draw their own
surface (they expose one full-screen node — use app intents or coordinates),
or anything where the device cannot be plugged in / paired over adb.

## The loop

```bash
hpctl devices                      # exactly one phone must be attached
hpctl state                        # which app is in front (0.1s, no dump)
hpctl obs                          # screen as text: [ref] role "label" @x,y id=...
hpctl tap --text "Search"          # act on the label; prints the resulting screen
hpctl type "coffee" --into "Search"
```

Repeat: read → pick a label → act → the printed screen is the verification. The
screen printed after an action is read *after* the UI sequence number changed,
so it is the post-action state, not the one before it — do not call `obs` again
just to check.

## Picking a target

1. `--text "Exact label"` or `--id send_button` — stable, survives layout shifts.
2. `--contains "kirim"` — when the label changes slightly (counts, names).
3. `-r N` — only when you just ran `obs` and the screen has not changed since.
4. Several elements match → it refuses and lists them: add `--index N` or
   `--id`. Never guess a coordinate instead.
5. `obs` flags risky elements with `⚠risky:<word>` — read it before tapping.

## The risk gate is not optional

`tap`, `type` and `launch` exit with code **3** and print `NEEDS_APPROVAL:`
when the target looks irreversible or financial (send, pay, delete, post,
install; banking/e-wallet/marketplace packages).

When that happens:

1. Stop. Do not retry with `--yes` on your own.
2. Tell the user what you are about to do and to which app.
3. Re-run the identical command with `--yes` **only after the user says yes.**

The gate cannot tell whether the human agreed — it only makes an accidental
irreversible tap harder. Approval lives in the conversation, not in the binary.

## Speed: the companion app

Reads cost 0.04–0.09 s with the companion installed and enabled, 1.7–1.8 s
without. If the user has not installed it, `hpctl` still works — it falls back
to `uiautomator` by itself. A stale or missing `tree.json` (older than 4 s) is
never used, so there is nothing to "refresh" by hand.

The companion also makes `hpctl type` accept any script (emoji, non-Latin) by
writing with `ACTION_SET_TEXT` instead of `input text`, which is ASCII only.

## Pitfalls that cost real time

- **Screen off = empty tree.** `adb shell svc power stayon usb`, or
  `hpctl key 224`, then check `hpctl state`.
- **Another accessibility service can swallow the dump.** Symptom: `obs`
  returns ~13 system-UI elements even though another app is clearly on screen.
  Disable that service (with the user's permission) or install the companion.
- **Secure windows** (lock screen, some banking apps) refuse dumps entirely.
- **Phone clock ≠ host clock.** The offset is measured once and cached in
  `~/.cache/hpctl/clock.json` for 5 minutes. If reads suddenly always take the
  slow path, delete that file before assuming the companion broke.
- **Slow screens need `wait`.** After tapping into a heavy screen, prefer
  `hpctl wait --text "…" --timeout 10` over sleeping.
- **Several phones attached** → it refuses to pick. Pass `--device SERIAL`
  or set `HPCTL_DEVICE`.

## Scripting

- Add `--quiet` to suppress the screen dump; keep it while exploring.
- One shell call can chain steps, but check the exit code: `3` means the gate
  fired, `1` means the element was not found (the error prints the whole screen
  so you can re-target).
- `hpctl shot out.png` when text is not enough; look at the picture instead of
  guessing from the tree.
