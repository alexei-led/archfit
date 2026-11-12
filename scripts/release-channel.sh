#!/bin/sh
# release-channel.sh TAG — the one tag rule of the release workflow.
#
# Prints "final" for vX.Y.Z and "prerelease" for vX.Y.Z-rc.N. Only a final tag
# moves the image tag "latest", becomes the latest GitHub release, and updates
# the Homebrew formula. Any other tag shape exits 2, so a tag the workflow
# trigger admits but this rule does not stops the run before anything publishes.
set -eu

refuse() {
	echo "release-channel.sh: $1" >&2
	exit 2
}

[ "$#" -eq 1 ] || refuse "usage: release-channel.sh TAG"

# grep matches line by line, so refuse a newline, or any character no rule uses, first.
case "$1" in
*[!v0-9.rc-]*) refuse "'$1' is neither vX.Y.Z nor vX.Y.Z-rc.N" ;;
esac

if printf '%s\n' "$1" | grep -Eqx 'v[0-9]+\.[0-9]+\.[0-9]+'; then
	echo final
elif printf '%s\n' "$1" | grep -Eqx 'v[0-9]+\.[0-9]+\.[0-9]+-rc\.[0-9]+'; then
	echo prerelease
else
	refuse "'$1' is neither vX.Y.Z nor vX.Y.Z-rc.N"
fi
