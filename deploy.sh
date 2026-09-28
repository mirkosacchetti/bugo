#!/bin/sh
# Pubblica il blog su https://blog.random.cards: genera il sito dai post nelle
# note (~/Notes/bugo/posts) e lo committa nel repo di pubblicazione
# mirkosacchetti/blog.random.cards (branch main, GitHub Pages con dominio
# personalizzato), conservandone CNAME e README.
# Override: BUGO_TARGET (repo), BUGO_BRANCH, BUGO_BASE (prefisso URL, vuoto
# alla radice del dominio), BUGO_POSTS (cartella dei post).
set -eu

repo=$(cd "$(dirname "$0")" && pwd)
cd "$repo"
target="${BUGO_TARGET:-git@github.com:mirkosacchetti/blog.random.cards.git}"
branch="${BUGO_BRANCH:-main}"
export BUGO_BASE="${BUGO_BASE-}"

rm -rf public/index.html public/post
go run . pub

out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT
git clone -q --depth 1 -b "$branch" "$target" "$out"
# Sostituisce il sito, lasciando la configurazione del repo di pubblicazione.
find "$out" -mindepth 1 -maxdepth 1 ! -name .git ! -name CNAME ! -name README.md \
    -exec rm -rf {} +
cp -R public/. "$out"
touch "$out/.nojekyll"
cd "$out"
git add -A
if git diff --cached --quiet; then
    echo "nessuna modifica da pubblicare"
else
    git commit -qm "deploy $(date '+%Y-%m-%d %H:%M')"
    git push -q origin "$branch"
fi
echo "pubblicato: $(ls "$repo/public/post" | wc -l | tr -d ' ') post"
