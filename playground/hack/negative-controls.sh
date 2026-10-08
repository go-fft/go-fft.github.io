#!/usr/bin/env bash
# Copyright (c) the go-fft authors.
# SPDX-License-Identifier: BSD-3-Clause
#
# A green lint only proves the lint is SILENT on this tree, not that it can
# still bite. For each guard, inject the exact leak it exists to catch into the
# real scene, and require it to fail; then restore the file and require green.
#
#   bricolint: a raw painter drawing primitive in the scene's Draw
#   mvvmlint:  a direct write to a widget state field
#
# usage: BRICOLINT=... MVVMLINT=... bash hack/negative-controls.sh
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
target="$root/scene.go"
anchor='s.root.Draw(painter.NewPixelPainter(buf, s.w, s.h), s.theme)'
grep -qF "$anchor" "$target" || { echo "anchor not found in scene.go: update this script" >&2; exit 2; }

backup="$(mktemp)"; cp "$target" "$backup"
trap 'cp "$backup" "$target"; rm -f "$backup"' EXIT

vet() { ( cd "$root" && GOWORK=off go vet -vettool="$1" ./... ) >/dev/null 2>&1; echo $?; }

check() { # name tool injected-line
  local name=$1 tool=$2 inject=$3
  cp "$backup" "$target"
  [ "$(vet "$tool")" = 0 ] || { echo "FAIL: $name is not green on the clean tree" >&2; exit 1; }
  awk -v a="$anchor" -v i="$inject" '{print} index($0,a){print i}' "$backup" > "$target"
  [ "$(vet "$tool")" != 0 ] || { echo "FAIL: $name stayed green with: $inject" >&2; exit 1; }
  cp "$backup" "$target"
  [ "$(vet "$tool")" = 0 ] || { echo "FAIL: $name not green after restoring" >&2; exit 1; }
  echo "ok: $name bites on the injected leak and is silent otherwise"
}

check bricolint "$BRICOLINT" '	painter.NewPixelPainter(buf, s.w, s.h).FillRect(painter.Rect{}, painter.RGBA{})'
check mvvmlint "$MVVMLINT" '	s.preset.Options = nil'
