#!/bin/sh
# Create a release tag with semtag. Run it through make:
#
#   make next-version SCOPE=minor           print the next final version
#   make release SCOPE=minor                tag and push vX.Y.Z (final)
#   make prerelease SCOPE=minor TYPE=rc     tag and push vX.Y.Z-rc.N (test version)
#
# Pushing the tag starts .github/workflows/release.yml. Final versions publish
# the Homebrew cask; pre-releases (alpha, beta, rc) do not.
#
# Environment:
#   SEMTAG  semtag executable (default: semtag).
#   GO      go executable (default: go).
#
# Every argument is checked against a fixed list before it reaches a command,
# and commands run with discrete arguments, never through a shell string.

set -eu

SEMTAG="${SEMTAG:-semtag}"
GO="${GO:-go}"
REMOTE="origin"
MAIN_BRANCH="main"

# semtag orders pre-release names as text, so rc.10 sorts before rc.9.
MAX_PRERELEASE=9

err() {
	echo "release: $*" >&2
	exit 1
}

usage() {
	err "usage: release.sh next <scope> | final <scope> | pre <scope> <type>   (scope: major|minor|patch, type: alpha|beta|rc)"
}

need() {
	command -v "$1" >/dev/null 2>&1 || err "missing required command: $1"
}

check_scope() {
	case "$1" in
	major | minor | patch) ;;
	"") err "SCOPE is required: major, minor, or patch" ;;
	*) err "invalid SCOPE: $1 (want major, minor, or patch)" ;;
	esac
}

# semtag_command maps a pre-release type to the semtag command that creates it.
semtag_command() {
	case "$1" in
	alpha) echo alpha ;;
	beta) echo beta ;;
	rc) echo candidate ;;
	"") err "TYPE is required: alpha, beta, or rc" ;;
	*) err "invalid TYPE: $1 (want alpha, beta, or rc)" ;;
	esac
}

# next_version prints the next final version (with the leading v). -f lets it
# run from any branch because nothing is tagged with -o.
next_version() {
	"$SEMTAG" final -s "$1" -o -f
}

require_clean_tree() {
	[ -z "$(git status --porcelain)" ] || err "working tree has uncommitted changes"
}

fetch_remote() {
	git fetch --quiet --tags "$REMOTE" || err "could not fetch $REMOTE"
}

cmd_next() {
	check_scope "$1"
	next_version "$1"
}

# cmd_final tags the next final version from main after checking that the
# changelog already has its section.
cmd_final() {
	scope="$1"
	check_scope "$scope"

	branch=$(git rev-parse --abbrev-ref HEAD)
	[ "$branch" = "$MAIN_BRANCH" ] || err "final versions are tagged from $MAIN_BRANCH (current branch: $branch)"
	require_clean_tree
	fetch_remote
	[ "$(git rev-parse HEAD)" = "$(git rev-parse "$REMOTE/$MAIN_BRANCH")" ] ||
		err "$MAIN_BRANCH is not at $REMOTE/$MAIN_BRANCH; pull or push first"

	version=$(next_version "$scope")
	"$GO" run ./tools/changelog notes "$version" >/dev/null ||
		err "CHANGELOG.md has no section for $version; run: make changelog-release SCOPE=$scope, then merge it"

	echo "Tagging final version $version"
	"$SEMTAG" final -s "$scope"
}

# cmd_pre tags the next pre-release of the next final version. semtag always
# numbers a pre-release .1, so the number is counted here and passed with -v.
cmd_pre() {
	scope="$1"
	type="$2"
	check_scope "$scope"
	command=$(semtag_command "$type")

	require_clean_tree
	fetch_remote
	[ -n "$(git branch -r --contains HEAD)" ] || err "HEAD is not on $REMOTE; push the commit first"

	base=$(next_version "$scope")
	last=$(git tag -l "$base-$type.*" | sed "s/^$base-$type\.//" | sort -n | tail -n 1)
	number=$((${last:-0} + 1))
	[ "$number" -le "$MAX_PRERELEASE" ] ||
		err "$base-$type.$number would sort before $base-$type.$MAX_PRERELEASE in semtag; cut a final version instead"

	version="$base-$type.$number"
	echo "Tagging pre-release $version"
	"$SEMTAG" "$command" -s "$scope" -v "$version"
}

main() {
	[ $# -ge 1 ] || usage
	mode="$1"
	shift

	need git
	need "$SEMTAG"

	case "$mode" in
	next)
		[ $# -eq 1 ] || usage
		cmd_next "$1"
		;;
	final)
		[ $# -eq 1 ] || usage
		need "$GO"
		cmd_final "$1"
		;;
	pre)
		[ $# -eq 2 ] || usage
		cmd_pre "$1" "$2"
		;;
	*) usage ;;
	esac
}

main "$@"
