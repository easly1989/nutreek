#!/usr/bin/env bash
# Records a stable release in versions.json on the orphan `site-data` branch
# (#167). Instances read it from raw.githubusercontent.com to tell their owner
# that a newer Nutreek is out, and the website can read it too:
#
#   { "stable": { "version": "v2.1.0", "image": "ghcr.io/easly1989/nutreek:latest", "sha": "...", "published": "..." } }
#
# Usage: publish-version.sh <version>
#   An empty <version> (a manual Release run with no version) keeps the
#   recorded version and only moves its sha and date.
#
# Needs GITHUB_TOKEN with contents: write. A push made with GITHUB_TOKEN starts
# no workflow, and nothing listens on site-data anyway. A rejected push (two
# publishes racing) starts again from a fresh clone.
set -euo pipefail

version=${1-}
image="ghcr.io/${GITHUB_REPOSITORY}:latest"

# SITE_DATA_REMOTE points it at another repository, for trying it locally.
remote=${SITE_DATA_REMOTE:-"https://x-access-token:${GITHUB_TOKEN}@github.com/${GITHUB_REPOSITORY}.git"}
published=$(date -u +%Y-%m-%dT%H:%M:%SZ)
dir=$(mktemp -d)

for attempt in 1 2 3 4 5; do
  rm -rf "$dir" && mkdir -p "$dir"
  if git ls-remote --exit-code --heads "$remote" site-data >/dev/null; then
    git clone --quiet --depth 1 --branch site-data "$remote" "$dir"
  else
    # First publish ever: start the branch with no history from main.
    git -C "$dir" init --quiet
    git -C "$dir" checkout --quiet --orphan site-data
    git -C "$dir" remote add origin "$remote"
    echo '{}' > "$dir/versions.json"
  fi

  jq --arg v "$version" --arg img "$image" --arg sha "$GITHUB_SHA" --arg at "$published" '
    .stable = ((.stable // {}) + {image: $img, sha: $sha, published: $at}
               + (if $v == "" then {} else {version: $v} end))
  ' "$dir/versions.json" > "$dir/versions.json.new"
  mv "$dir/versions.json.new" "$dir/versions.json"

  git -C "$dir" add versions.json
  if git -C "$dir" diff --cached --quiet; then
    echo "versions.json already records this publish"
    exit 0
  fi
  git -C "$dir" -c user.name="github-actions[bot]" \
    -c user.email="41898282+github-actions[bot]@users.noreply.github.com" \
    commit --quiet -m "versions: stable ${version:-$GITHUB_SHA}"
  if git -C "$dir" push --quiet origin site-data; then
    cat "$dir/versions.json"
    exit 0
  fi
  echo "push rejected (attempt $attempt), starting again from a fresh clone" >&2
  sleep $((attempt * 5))
done
echo "could not update versions.json" >&2
exit 1
