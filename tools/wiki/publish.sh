#!/usr/bin/env bash
#
# Publish docs/wiki/pages to both wiki repos.
#
# The wiki source lives in the product repo so it is reviewed in normal commits
# and cannot be edited in two places. The two wiki repos are outputs. Nothing
# syncs them on its own: GitLab's repository mirroring copies branches, tags and
# commits of the PROJECT repo and has never covered wikis (gitlab#37049), and
# GitHub has no mirroring at all. So this script is the only path, and running it
# is what "reflected into GitHub" means.
#
# Three things differ between the hosts and each one silently publishes nothing
# if you get it wrong:
#
#   branch     GitLab wiki is main, GitHub wiki is master and cannot be changed
#   landing    GitLab serves the page named home, GitHub the page named Home
#   sidebar    GitLab reads _sidebar.md, GitHub reads _Sidebar.md
#
# One source tree, renamed on the way out, rather than two copies to keep level.
#
# The GitHub wiki repo does not exist until somebody saves one page in the
# browser. Until then every push fails with "Repository not found" and there is
# no API that can do it for you.
#
# Usage:
#   tools/wiki/publish.sh                 lint, then publish to both
#   tools/wiki/publish.sh --dry-run       build the trees, show the diff, push nothing
#   tools/wiki/publish.sh --only gitlab   one host
#   tools/wiki/publish.sh -m "message"    commit message for both
#   tools/wiki/publish.sh --no-lint       skip the lint (do not)

set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SRC="$REPO/docs/wiki/pages"
WORK="$REPO/.wiki-build"

GITLAB_URL="git@gitlab.com:fu-bar/agentbox.wiki.git"
GITHUB_URL="git@github.com:borismilner/agentbox.wiki.git"

DRY=0
LINT=1
ONLY=""
MESSAGE=""

while [ $# -gt 0 ]; do
	case "$1" in
	--dry-run) DRY=1 ;;
	--no-lint) LINT=0 ;;
	--only)
		ONLY="${2:?--only needs gitlab or github}"
		shift
		;;
	-m | --message)
		MESSAGE="${2:?-m needs a message}"
		shift
		;;
	-h | --help)
		sed -n '3,32p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
		exit 0
		;;
	*)
		echo "publish.sh: unknown argument $1" >&2
		exit 2
		;;
	esac
	shift
done

[ -d "$SRC" ] || {
	echo "publish.sh: no wiki source at $SRC" >&2
	exit 2
}

if [ "$LINT" = 1 ]; then
	python3 "$REPO/tools/wiki/lint.py" || {
		echo "publish.sh: lint failed, nothing pushed" >&2
		exit 1
	}
fi

if [ -z "$MESSAGE" ]; then
	MESSAGE="docs(wiki): sync from $(git -C "$REPO" rev-parse --short HEAD)"
fi

# stage HOST URL BRANCH: bring the clone up to date, replace its pages with the
# source tree renamed for this host, and push if anything actually changed.
stage() {
	local host="$1" url="$2" branch="$3" dir="$WORK/$1"

	if [ ! -d "$dir/.git" ]; then
		mkdir -p "$WORK"
		echo "==> cloning $host wiki"
		git clone --quiet "$url" "$dir" 2>&1 | grep -v 'warning: You appear to have cloned an empty repository' || true
	fi

	git -C "$dir" fetch --quiet origin || {
		echo "publish.sh: cannot reach the $host wiki repo at $url" >&2
		if [ "$host" = github ]; then
			echo "            the GitHub wiki repo does not exist until one page is saved at" >&2
			echo "            https://github.com/borismilner/agentbox/wiki" >&2
		fi
		return 1
	}

	if git -C "$dir" rev-parse --verify --quiet "origin/$branch" >/dev/null; then
		git -C "$dir" checkout --quiet -B "$branch" "origin/$branch"
		git -C "$dir" reset --hard --quiet "origin/$branch"
	else
		# Empty wiki repo: no commit to branch from yet.
		git -C "$dir" symbolic-ref HEAD "refs/heads/$branch"
	fi

	# Replace the published pages wholesale so a page deleted from source is
	# deleted from the wiki. Everything else in the repo is left alone, which is
	# how GitLab's own .gitlab/redirects.yml survives a publish.
	find "$dir" -maxdepth 1 -name '*.md' -delete
	cp "$SRC"/*.md "$dir/"

	case "$host" in
	gitlab)
		[ -f "$dir/home.md" ] || { [ -f "$dir/Home.md" ] && mv "$dir/Home.md" "$dir/home.md"; }
		[ -f "$dir/_Sidebar.md" ] && mv "$dir/_Sidebar.md" "$dir/_sidebar.md"
		;;
	github)
		[ -f "$dir/Home.md" ] || { [ -f "$dir/home.md" ] && mv "$dir/home.md" "$dir/Home.md"; }
		[ -f "$dir/_sidebar.md" ] && mv "$dir/_sidebar.md" "$dir/_Sidebar.md"
		;;
	esac

	git -C "$dir" add -A
	if git -C "$dir" diff --cached --quiet; then
		echo "==> $host: already up to date"
		return 0
	fi

	echo "==> $host: $(git -C "$dir" diff --cached --name-status | wc -l) file(s) changed"
	git -C "$dir" diff --cached --stat | sed 's/^/    /'

	if [ "$DRY" = 1 ]; then
		echo "    (dry run, not pushed)"
		return 0
	fi

	git -C "$dir" -c user.name="$(git -C "$REPO" config user.name)" \
		-c user.email="$(git -C "$REPO" config user.email)" \
		commit --quiet -m "$MESSAGE"
	git -C "$dir" push --quiet origin "HEAD:$branch"
	echo "    pushed to $branch"
}

rc=0
case "$ONLY" in
gitlab) stage gitlab "$GITLAB_URL" main || rc=1 ;;
github) stage github "$GITHUB_URL" master || rc=1 ;;
"")
	stage gitlab "$GITLAB_URL" main || rc=1
	stage github "$GITHUB_URL" master || rc=1
	;;
*)
	echo "publish.sh: --only takes gitlab or github, not $ONLY" >&2
	exit 2
	;;
esac

if [ "$DRY" = 0 ] && [ "$rc" = 0 ]; then
	echo
	echo "https://gitlab.com/fu-bar/agentbox/-/wikis/home"
	echo "https://github.com/borismilner/agentbox/wiki"
fi
exit "$rc"
