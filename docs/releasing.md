# Releasing

A release is a decision, not a consequence of pushing. Nothing publishes itself:
`main` moving proves the tree is good, and `make release` is the separate act of
saying the world should have this one.

```sh
make release VERSION=v0.2.0
```

That is the whole convention. The rest of this page is what it does, why the
order matters, and what to do when it stops halfway.

## What a version number means here

`v0.MINOR.PATCH` while the product is pre-1.0, and the binary itself carries no
version at all - identity inside a build is the toolchain's VCS stamp
(`agentbox version`), and the tag is a label applied from outside. So the number
is a statement about the release, not about the code:

| Bump | When |
|---|---|
| PATCH | fixes and refinements only; nothing a user has to read about |
| MINOR | a new surface, tool, or behaviour somebody would notice |
| MAJOR | reserved. 1.0 is a decision about the product, not about a diff |

`v0.1.0` (2026-08-12) is the first. Before it there were no tags and no
downloads, which is why a few older pages still say there is nothing to download.

## What `make release` refuses to do

Each of these is a way to publish something nobody can reproduce, so it stops
rather than warns:

- a `VERSION` that is not `vX.Y.Z`, including the default (which is a commit hash,
  and would make a tag nobody can read)
- a dirty tree
- a branch that is not `main`
- a tag that already exists
- a `HEAD` that either remote does not have yet - a tag on an unpushed commit
  builds something nobody can check out

Then it runs `make check` and `make check-dist`, tags, and pushes.

## Why gitlab before github

**GitLab first, always.** GitHub Actions builds the release and then asks
GitLab's API to create a matching one; that call fails if the tag is not on
GitLab yet. It is also the direction the mirror runs (gitlab to github), so the
same order keeps the mirror from having an opinion.

The GitHub push is what starts the build - `.github/workflows/release.yml` gates
the tag's own commit, packages it with `make dist`, publishes, then hands the
identical bytes to GitLab over its API. GitLab builds nothing: its free tier
meters CI at 400 minutes a month, and GitHub's runners are unmetered on a public
repo.

## Only the newest release is kept

Both sides prune to one. What is deleted is the release object, its download and
its notes; **tags are never deleted**, so `git checkout v0.1.0` keeps working and
a pruned release can be rebuilt from its tag with one command. On GitLab the
generic-package versions are pruned too, since those consume the free storage
quota.

This is a "for now" policy - it keeps the releases page down to the one thing
anybody wants, which is the current download.

## The download link never changes

```
https://github.com/borismilner/agentbox/releases/latest/download/agentbox-linux-amd64.tar.gz
```

`/releases/latest/download/` resolves to the newest release and keys on the
FILENAME, which is why `make dist` names the asset without a version in it. Put a
version in the name and every link to it goes stale on the next release.

## When it fails halfway

The tag is pushed before the build runs, so a failed release leaves a tag behind
and no download. Nothing is corrupt; the fix depends on where it stopped.

**The build failed.** Fix the cause, then re-run the same tag rather than burning
a number:

```sh
gh run rerun --repo borismilner/agentbox --failed
```

If the fix is a code change, the tag has to move to the new commit - delete it on
both remotes and cut it again:

```sh
git tag -d v0.2.0
git push origin :refs/tags/v0.2.0
git push github :refs/tags/v0.2.0
make release VERSION=v0.2.0
```

**The GitLab step was skipped.** It says so and exits 0 on purpose: a missing
mirror is not a reason to fail a release that is already published. It needs a
GitLab personal access token with the `api` scope:

```sh
# create at https://gitlab.com/-/user_settings/personal_access_tokens
gh secret set GITLAB_TOKEN --repo borismilner/agentbox
gh run rerun --repo borismilner/agentbox --failed   # or re-run the whole job
```

**A commit touching `.github/workflows/` will not mirror.** GitHub refuses a
workflow-file change pushed by a token without the `workflow` scope, and the
mirror pushes over HTTPS with a PAT. Push such a commit to `github` over SSH
first; the mirror's own push is then a no-op, which nothing refuses.
