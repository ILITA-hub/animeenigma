#!/usr/bin/env bash
#
# AnimeEnigma — Walpurgis no Kaiten release watcher (owner reminder).
#
# Weekly cron: reminds the owner (Telegram admin chat, maintenance bot) to
# download «Puella Magi Madoka Magica: Walpurgisnacht Rising»
# (劇場版 魔法少女まどか☆マギカ〈ワルプルギスの廻天〉, shikimori/MAL 48820)
# once it is actually obtainable. Two independent one-shot signals:
#
#   1. premiere  — catalog status flips off "announced" OR aired_on
#                  (2026-08-28 JP theatrical) has passed. FYI ping only:
#                  torrents at this point are camrips.
#   2. torrent   — library search (Jackett→Nyaa/AnimeTosho) surfaces a
#                  non-camrip >=1080p (or >=2 GiB) release. Actionable ping:
#                  time to enqueue the download.
#
# Each signal notifies ONCE (state file), so the cron can run forever without
# spam. After BOTH have fired the script exits immediately doing nothing.
#
# Requested by the owner 2026-07-03 («напоминание скачать, когда выйдет»).
# Installed to: /usr/local/bin/animeenigma-walpurgis-watch.sh (mode 755)
# Cron:         /etc/cron.d/animeenigma-walpurgis-watch (weekly)
# State:        /var/lib/animeenigma/walpurgis-watch.state
# Log:          /var/log/animeenigma-walpurgis-watch.log
#
set -uo pipefail

ANIME_UUID="7d3d27d8-f76b-4174-a9ee-a0dd152c6fc9" # catalog row (shiki 48820)
AIRED_ON_EPOCH=$(date -d "2026-08-28" +%s)
STATE_DIR="/var/lib/animeenigma"
STATE="$STATE_DIR/walpurgis-watch.state"
MAINT_ENV="/data/animeenigma/docker/maintenance.env"
CATALOG="http://127.0.0.1:8081"
LIBRARY="http://127.0.0.1:8089"

mkdir -p "$STATE_DIR"
touch "$STATE"

has(){ grep -q "^$1$" "$STATE"; }
mark(){ echo "$1" >>"$STATE"; }
ts(){ date -u +%FT%TZ; }

# Both signals already fired — nothing left to watch.
if has premiere_notified && has torrent_notified; then exit 0; fi

tg_send(){
  # shellcheck disable=SC1090
  set -a; . "$MAINT_ENV"; set +a
  [ -n "${TELEGRAM_BOT_TOKEN:-}" ] && [ -n "${TELEGRAM_ADMIN_CHAT_ID:-}" ] || {
    echo "$(ts) ERROR: telegram creds missing in $MAINT_ENV" >&2; return 1; }
  curl -sf --max-time 20 -X POST \
    "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/sendMessage" \
    -d "chat_id=${TELEGRAM_ADMIN_CHAT_ID}" \
    --data-urlencode "text=$1" >/dev/null
}

# ---- Signal 1: premiere ----------------------------------------------------
if ! has premiere_notified; then
  status=$(curl -sf --max-time 15 "$CATALOG/api/anime/$ANIME_UUID" \
    | jq -r '.data.status // empty' 2>/dev/null)
  now=$(date +%s)
  if { [ -n "$status" ] && [ "$status" != "announced" ]; } || [ "$now" -ge "$AIRED_ON_EPOCH" ]; then
    tg_send "🎬 Madoka Magica: Walpurgisnacht Rising (ワルプルギスの廻天) — премьера состоялась (статус: ${status:-past aired_on}). Нормального рипа ещё нет — этот бот продолжит следить за торрентами и напомнит, когда можно будет скачать." \
      && { mark premiere_notified; echo "$(ts) premiere notified (status=$status)"; }
  fi
fi

# ---- Signal 2: proper torrent ----------------------------------------------
if ! has torrent_notified; then
  found=""
  for q in "Walpurgis no Kaiten" "Madoka Magica Walpurgisnacht Rising"; do
    hits=$(curl -sf --max-time 120 --get "$LIBRARY/api/library/search" \
        --data-urlencode "q=$q" --data-urlencode "limit=30" \
      | jq -r '.data.releases[]?
          | select(.title | test("walpurgis|廻天"; "i"))
          | select(.title | test("camrip|hdcam|hdts|hdtc|[ .[(]ts[ .\\])]|\\bcam\\b"; "i") | not)
          | select((.quality == "1080p" or .quality == "2160p") or (.size_bytes >= 2147483648))
          | "\(.quality // "?") \(.size_bytes / 1073741824 * 10 | round / 10)GB [\(.source)] \(.title)"' \
        2>/dev/null | head -5)
    [ -n "$hits" ] && { found="$hits"; break; }
  done
  if [ -n "$found" ]; then
    tg_send "⬇️ Madoka Magica: Walpurgisnacht Rising — появились нормальные торренты, можно скачивать (оригинал в макс качестве):
$found

Напоминание по просьбе от 2026-07-03. Скажи Claude Code: «скачай Walpurgis no Kaiten (shiki 48820) в library» — и не забудь episode=1 в library_jobs (фильм, имя файла без номера эпизода)." \
      && { mark torrent_notified; echo "$(ts) torrent notified"; }
  fi
fi

exit 0
