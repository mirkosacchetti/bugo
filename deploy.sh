#!/bin/sh
# Pubblica il blog su GitHub Pages: genera il sito dai post nelle note
# (~/Notes/bugo/posts) e lo spinge, da solo, sul branch gh-pages.
# Override: BUGO_POSTS (cartella dei post), BUGO_BASE (prefisso URL, "" con un
# dominio personalizzato).
set -eu

repo=$(cd "$(dirname "$0")" && pwd)
cd "$repo"
export BUGO_BASE="${BUGO_BASE-/bugo}"
remote=$(git remote get-url origin)

rm -rf public/index.html public/post
go run . pub

out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT
cp -R public/. "$out"
touch "$out/.nojekyll"
cd "$out"
git init -q -b gh-pages
git add -A
git commit -qm "deploy $(date '+%Y-%m-%d %H:%M')"
git push -qf "$remote" gh-pages
echo "pubblicato: $(ls "$repo/public/post" | wc -l | tr -d ' ') post"
