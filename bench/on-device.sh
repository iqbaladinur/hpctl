#!/usr/bin/env bash
# End-to-end timings against the phone that is plugged in right now.
# Prints a markdown table, so the result can go straight into the README.
#
#   ./bench/on-device.sh [runs]
#
# Everything measured is a read: obs (find the screen), state (which app),
# wait (find one control, polling). Nothing taps.
set -euo pipefail

RUNS=${1:-10}
BIN=${HPCTL:-hpctl}
command -v "$BIN" >/dev/null || BIN=./hpctl

mapfile -t DEVICES < <(adb devices | awk 'NR>1 && $2=="device" {print $1}')
if [ "${#DEVICES[@]}" -ne 1 ]; then
  echo "need exactly one device attached, found ${#DEVICES[@]}: ${DEVICES[*]:-none}" >&2
  exit 1
fi
SERIAL=${DEVICES[0]}
ADB=(adb -s "$SERIAL")

echo "device: $SERIAL"
"${ADB[@]}" shell svc power stayon usb >/dev/null 2>&1 || true
"${ADB[@]}" shell input keyevent 224 >/dev/null 2>&1 || true
sleep 1

# Warm the companion before timing. After (re)enabling the service, or after a
# uiautomator run, it may not have written a tree yet and every read would fall
# back to the 2.5 s path — which would look like hpctl is slow.
ms_once() { local s=$(date +%s%N); "$BIN" obs --quiet >/dev/null 2>&1; echo $(( ($(date +%s%N)-s)/1000000 )); }
warm=0
for i in $(seq 12); do
  if [ "$(ms_once)" -lt 500 ]; then warm=1; echo "companion warm after ${i}s"; break; fi
  if [ "$i" -eq 4 ]; then
    echo "companion cold — restarting the accessibility service"
    "${ADB[@]}" shell settings delete secure enabled_accessibility_services >/dev/null
    "${ADB[@]}" shell settings put secure accessibility_enabled 0 >/dev/null
    "${ADB[@]}" shell settings put secure enabled_accessibility_services id.iqbaldev.hpctl.companion/.TreeDumpService >/dev/null
    "${ADB[@]}" shell settings put secure accessibility_enabled 1 >/dev/null
  fi
  sleep 1
done
[ "$warm" -eq 1 ] || echo "WARNING: companion still cold — the rows below measure the fallback path"

model=$("${ADB[@]}" shell getprop ro.product.model | tr -d '\r')
release=$("${ADB[@]}" shell getprop ro.build.version.release | tr -d '\r')
echo "model: $model (Android $release)"

# ms for one command, $RUNS times; prints median / min / max and how many runs
# exited non-zero. A failed lookup returns fast and would otherwise look great.
timeit() {
  local label=$1; shift
  local -a vals=()
  local fails=0
  for _ in $(seq "$RUNS"); do
    local start end rc
    start=$(date +%s%N)
    "$@" >/dev/null 2>&1; rc=$?
    end=$(date +%s%N)
    if [ "$rc" -ne 0 ]; then fails=$((fails + 1)); fi
    vals+=($(( (end - start) / 1000000 )))
  done
  printf '%s\n' "${vals[@]}" | sort -n | awk -v l="$label" -v f="$fails" -v n="$RUNS" '
    {v[NR]=$1}
    END {
      printf "| %s | %d | %d | %d | %d/%d |\n", l, v[int((NR+1)/2)], v[1], v[NR], f, n
    }'
}

echo
# --slow runs last: on some devices uiautomator tears down the accessibility
# connection, which would make every later companion read fall back.
echo "| operation | median ms | min ms | max ms | failed |"
echo "|---|---|---|---|---|"
timeit "state (which app, dumpsys only)" "$BIN" state
timeit "obs (companion file)" "$BIN" obs --quiet

# Find-one-control: poll until a label that is already on screen matches.
target=${HPCTL_BENCH_TARGET:-}
if [ -z "$target" ]; then
  target=$("$BIN" obs | sed -n 's/^\[[0-9]*\][^"]*"\([^"]\{2,20\}\)".*/\1/p' | head -1)
fi
if [ -n "$target" ]; then
  echo "wait target: \"$target\""
  timeit "wait --text \"$target\" (find one control)" "$BIN" wait --text "$target" --timeout 5s
fi

# Optional tap timing. Off by default: tapping changes the screen. Set it to a
# label you are fine tapping (a tab, a menu entry) — the risk gate still applies.
if [ -n "${HPCTL_BENCH_TAP:-}" ]; then
  timeit "tap \"$HPCTL_BENCH_TAP\" --quiet (inject, no wait)" \
    "$BIN" tap --quiet --text "$HPCTL_BENCH_TAP"
  timeit "tap \"$HPCTL_BENCH_TAP\" (inject + wait for redraw)" \
    "$BIN" tap --text "$HPCTL_BENCH_TAP"
fi
timeit "obs --slow (uiautomator dump, runs last)" "$BIN" obs --quiet --slow
