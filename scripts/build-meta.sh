#!/bin/sh
# build-meta.sh — build metadata for -ldflags (-X main.commit / -X main.date).
#
# Usage: sh scripts/build-meta.sh commit|date
#
# The date is the commit time in UTC, not the wall clock, so two builds of one
# commit print the same string. Prints "none" / "unknown" outside a git tree,
# the same fallbacks cmd/archfit/main.go carries.
set -eu

case "${1:-}" in
commit)
	git rev-parse --short HEAD 2>/dev/null || echo none
	;;
date)
	# --no-show-signature: a user's log.showSignature=true must not put gpg lines on stdout.
	TZ=UTC git log -1 --no-show-signature --format=%cd --date=format-local:%Y-%m-%dT%H:%M:%SZ HEAD 2>/dev/null || echo unknown
	;;
*)
	echo "usage: build-meta.sh commit|date" >&2
	exit 2
	;;
esac
