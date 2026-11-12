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

model=$("${ADB[@]}" shell getprop ro.product.model | tr -d '\r')
release=$("${ADB[@]}" shell getprop ro.build.version.release | tr -d '\r')
echo "model: $model (Android $release)"

# ms for one command, $RUNS times; returns median / min / max on one line.
timeit() {
  local label=$1; shift
  local -a vals=()
  for _ in $(seq "$RUNS"); do
    local start end
    start=$(date +%s%N)
    "$@" >/dev/null 2>&1 || { echo "FAILED: $label" >&2; return 1; }
    end=$(date +%s%N)
    vals+=($(( (end - start) / 1000000 )))
  done
  printf '%s\n' "${vals[@]}" | sort -n | awk -v l="$label" '
    {v[NR]=$1}
    END {printf "| %s | %d | %d | %d |\n", l, v[int((NR+1)/2)], v[1], v[NR]}'
}

echo
echo "| operation | median ms | min ms | max ms |"
echo "|---|---|---|---|"
timeit "state (which app, dumpsys only)" "$BIN" state
timeit "obs (companion file)" "$BIN" obs --quiet
timeit "obs --slow (uiautomator dump)" "$BIN" obs --quiet --slow

# Find-one-control: poll until a label that is already on screen matches.
target=${HPCTL_BENCH_TARGET:-}
if [ -z "$target" ]; then
  target=$("$BIN" obs | sed -n 's/^\[[0-9]*\][^"]*"\([^"]\{2,20\}\)".*/\1/p' | head -1)
fi
if [ -n "$target" ]; then
  echo "wait target: \"$target\""
  timeit "wait --text \"$target\" (find one control)" "$BIN" wait --text "$target" --timeout 5
fi
