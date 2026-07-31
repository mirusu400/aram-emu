#!/usr/bin/env bash

set -euo pipefail

if [[ "$#" -ne 1 ]]; then
  echo "usage: $0 <apk>" >&2
  exit 2
fi

apk="$1"
package="io.github.mirusu400.aram.nightly"
activity="${package}/io.github.mirusu400.aram.app.MainActivity"

test -f "${apk}"
test "$(adb shell getconf PAGE_SIZE | tr -d '\r')" = "16384"
adb shell getprop ro.product.cpu.abilist | tr -d '\r' | grep -F x86_64
adb install -r "${apk}"

adb logcat -c
adb shell am start -W -n "${activity}"

pid=""
for attempt in $(seq 1 10); do
  pid="$(adb shell pidof "${package}" | tr -d '\r')"
  if [[ -n "${pid}" ]]; then
    break
  fi
  sleep 1
done

if [[ -z "${pid}" ]]; then
  adb logcat -d -b crash -t 200 >&2
  echo "${package} did not remain running" >&2
  exit 1
fi
