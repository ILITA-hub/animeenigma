#!/usr/bin/env bash
# Phase 18 Plan 18-01 Task 3 — refresh services/scraper/testdata/gogoanime/* goldens
# from anitaku.to in one atomic invocation.
#
# Issue 5 (revision): script extracts data-video URLs from the captured episode
# page and routes each by host to the matching wrapper fetch, so all 8 fixtures
# refresh together. No manual bootstrap needed.
#
# Issue 12 (upstream-death recovery): `curl -f` halts the script on any 4xx/5xx
# from anitaku.to / vibeplayer.site / otakuhg.site / otakuvid.online. On halt,
# the executor must document the death in the phase SUMMARY and trigger a fresh
# pivot per .planning/phases/18-9anime/18-RESEARCH.md §Mirror Viability D1.
# Do NOT substitute synthetic fixtures — the offline-test contract requires
# real upstream captures.
set -euo pipefail

OUTDIR="services/scraper/testdata/gogoanime"
UA="Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36"
CURL=(curl -fsSL -A "$UA" -H 'Accept: text/html,application/xhtml+xml' --cookie-jar /dev/null --no-keepalive --max-time 10 --connect-timeout 5)

mkdir -p "$OUTDIR"

fetch() {
    local out="$1" url="$2"
    local referer="${3:-}"
    echo "fetching $url -> $out"
    if [[ -n "$referer" ]]; then
        "${CURL[@]}" -H "Referer: $referer" "$url" > "$OUTDIR/$out"
    else
        "${CURL[@]}" "$url" > "$OUTDIR/$out"
    fi
    echo "  captured: $out ($(wc -c <"$OUTDIR/$out") bytes)"
}

fetch search_attack_on_titan.html "https://anitaku.to/search.html?keyword=Attack+on+Titan"
fetch category_one_piece.html     "https://anitaku.to/category/one-piece"
fetch category_one_piece_dub.html "https://anitaku.to/category/one-piece-dub"
fetch one_piece_episode_1.html    "https://anitaku.to/one-piece-episode-1"

# Issue 5 — parse data-video URLs from the episode page and dispatch each by
# host to the matching wrapper fetch so all 3 embed wrappers refresh in the
# same atomic invocation.
EP_HTML="$OUTDIR/one_piece_episode_1.html"
extract_data_video() {
    grep -oE 'data-video="[^"]+"' "$EP_HTML" | sed -E 's/data-video="//; s/"$//'
}

VIBE_URL=""; STREAMHG_URL=""; EARNVIDS_URL=""
while IFS= read -r u ; do
    case "$u" in
        //*) u="https:$u" ;;  # protocol-relative
    esac
    host="$(echo "$u" | awk -F/ '{print $3}' | tr 'A-Z' 'a-z')"
    case "$host" in
        vibeplayer.site|*.vibeplayer.site) [[ -z "$VIBE_URL"     ]] && VIBE_URL="$u" ;;
        otakuhg.site|*.otakuhg.site)       [[ -z "$STREAMHG_URL" ]] && STREAMHG_URL="$u" ;;
        otakuvid.online|*.otakuvid.online) [[ -z "$EARNVIDS_URL" ]] && EARNVIDS_URL="$u" ;;
    esac
done < <(extract_data_video)

if [[ -z "$VIBE_URL$STREAMHG_URL$EARNVIDS_URL" ]]; then
    echo "ERROR: episode page contains zero recognized embed hosts — recapture required" >&2
    exit 1
fi

[[ -n "$VIBE_URL"     ]] && fetch vibeplayer_embed.html "$VIBE_URL"     "https://anitaku.to/"
[[ -n "$STREAMHG_URL" ]] && fetch streamhg_packed.html  "$STREAMHG_URL" "https://otakuhg.site/"
[[ -n "$EARNVIDS_URL" ]] && fetch earnvids_packed.html  "$EARNVIDS_URL" "https://otakuvid.online/"

# malsync_no_gogo.json — negative-cache exemplar (One Piece MAL ID 21).
"${CURL[@]}" "https://api.malsync.moe/mal/anime/21" > "$OUTDIR/malsync_no_gogo.json"

# Anonymization sweep — strip any leaked Set-Cookie / DDoS / CF / Bearer headers.
sed -i -E '/(Set-Cookie|__ddg2_|cf_clearance|Bearer )/d' "$OUTDIR"/*.html

if grep -rE '(Set-Cookie|__ddg2_|cf_clearance|Bearer )' "$OUTDIR/" ; then
    echo "ERROR: forbidden auth pattern in goldens" >&2 ; exit 1
fi
echo "all goldens captured + anonymized"
