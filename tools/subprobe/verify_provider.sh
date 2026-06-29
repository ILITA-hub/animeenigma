#!/usr/bin/env bash
# Standalone provider sub-delivery verifier.
# Resolves a provider's stream, inspects the `tracks` field (softsub signal),
# extracts mid-episode frames through the streaming proxy, and runs the
# tier-1+OCR detector to decide HARDSUB vs SOFTSUB/CLEAN.
#
# Usage: verify_provider.sh <uuid> <epnum> <provider> <category> [seek] [dur]
set -uo pipefail
UUID="$1"; EPNUM="$2"; PREFER="$3"; CAT="${4:-sub}"; SEEK="${5:-420}"; DUR="${6:-90}"
GW="${SUBPROBE_GATEWAY:-http://localhost:8000}"          # gateway base (override per env)
SELF="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"      # tool dir (python helpers live here)
OUTROOT="${SUBPROBE_OUT:-/tmp/subprobe-out}"              # frames/work scratch (NEVER the repo)
OUT="$OUTROOT/frames/verify_${PREFER}_${CAT}"; mkdir -p "$OUT"; rm -f "$OUT"/*.png 2>/dev/null
WORK="$OUTROOT/work"; mkdir -p "$WORK"

echo "############ PROVIDER=$PREFER CAT=$CAT ep=$EPNUM ############"
EPS=$(curl -s --max-time 180 "$GW/api/anime/$UUID/scraper/episodes?prefer=$PREFER&exclusive=true")
EPID=$(echo "$EPS"|python3 -c "import sys,json
try:
 d=json.load(sys.stdin);dd=d.get('data',d);eps=dd.get('episodes',[])
 m={e.get('number'):e.get('id') for e in eps}
 print(m.get($EPNUM, (eps[0]['id'] if eps else '')))
except Exception: print('')")
if [ -z "$EPID" ]; then echo "VERDICT $PREFER: UNRESOLVED (no episodes) :: $(echo "$EPS"|head -c 200)"; exit 0; fi
echo "  epid=$EPID"
SV=$(curl -s --max-time 180 "$GW/api/anime/$UUID/scraper/servers?episode=$EPID&prefer=$PREFER&exclusive=true")
SRV=$(echo "$SV"|python3 -c "import sys,json
try:
 d=json.load(sys.stdin);dd=d.get('data',d)
 s=[x for x in dd.get('servers',[]) if x.get('type')=='$CAT'] or dd.get('servers',[])
 print(s[0]['id'] if s else '')
except Exception: print('')")
if [ -z "$SRV" ]; then echo "VERDICT $PREFER: UNRESOLVED (no $CAT server) :: $(echo "$SV"|head -c 200)"; exit 0; fi
ENC=$(python3 -c "import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1],safe=''))" "$SRV")
ST=$(curl -s --max-time 200 "$GW/api/anime/$UUID/scraper/stream?episode=$EPID&server=$ENC&category=$CAT&prefer=$PREFER&exclusive=true")
echo "$ST" > "$WORK/verify_${PREFER}_${CAT}.json"

read TRACKS LANGS SRCURL REF TYPE <<<$(echo "$ST"|python3 -c "
import sys,json
d=json.load(sys.stdin);dd=d.get('data',d);s=dd.get('stream') or {}
src=(s.get('sources') or [{}])[0]
trk=[t for t in (s.get('tracks') or []) if (t.get('kind') in ('captions','subtitles'))]
langs=','.join(sorted({(t.get('label') or '?') for t in trk}))[:60] or '-'
ref=(s.get('headers') or {}).get('Referer','') or src.get('referer','') or '-'
print(len(trk), langs, src.get('url','-'), ref, src.get('type','-'))
" 2>/dev/null)
HOST=$(python3 -c "import urllib.parse,sys;u=sys.argv[1]; print(urllib.parse.urlparse(u).hostname or '-')" "$SRCURL" 2>/dev/null)
echo "  resolved: type=$TYPE host=$HOST tracks=$TRACKS [$LANGS] referer=${REF:0:40}"

# Build proxied URL (+referer if any)
PURL=$(python3 -c "
import sys,json,urllib.parse
d=json.load(open('$WORK/verify_${PREFER}_${CAT}.json'));dd=d.get('data',d);s=dd['stream'];src=s['sources'][0]
ref=(s.get('headers') or {}).get('Referer','') or src.get('referer','')
q={'url':src['url'],'exp':src.get('exp',''),'sig':src.get('sig','')}
if ref: q['referer']=ref
print('$GW/api/streaming/hls-proxy?'+urllib.parse.urlencode(q))
" 2>/dev/null)

# Extract frames. HLS -> rewrite playlist to absolute + AES/.jpg support. MP4 -> direct.
if echo "$TYPE$SRCURL" | grep -qiE 'm3u8|hls'; then
  curl -s --max-time 40 "$PURL" > "$WORK/${PREFER}_master.m3u8" 2>/dev/null
  python3 -c "
import re
t=open('$WORK/${PREFER}_master.m3u8').read()
t=t.replace('URI=\"/api/streaming/','URI=\"$GW/api/streaming/')
t=re.sub(r'^/api/streaming/','$GW/api/streaming/',t,flags=re.M)
open('$WORK/${PREFER}_local.m3u8','w').write(t)
"
  ffmpeg -allowed_extensions ALL -protocol_whitelist file,http,https,tcp,tls,crypto -ss "$SEEK" -i "$WORK/${PREFER}_local.m3u8" -t "$DUR" -vf "fps=1/6" -q:v 2 "$OUT/f_%03d.png" -y -loglevel error 2>/dev/null
else
  ffmpeg -ss "$SEEK" -i "$PURL" -t "$DUR" -vf "fps=1/6" -q:v 2 "$OUT/f_%03d.png" -y -loglevel error 2>/dev/null
fi
NF=$(ls "$OUT"/*.png 2>/dev/null | wc -l)
echo "  frames=$NF"

if [ "$NF" -gt 0 ]; then
  python3 "$SELF/verify_verdict.py" "$OUT" "$PREFER" "$TRACKS"
else
  if [ "${TRACKS:-0}" -gt 0 ] 2>/dev/null; then
    echo "VERDICT $PREFER: SOFTSUB (tracks=$TRACKS [$LANGS]; video unread) — clean video + soft subs"
  else
    echo "VERDICT $PREFER: UNREAD (no frames, no tracks) host=$HOST — sid/CDN gated, inconclusive"
  fi
fi
