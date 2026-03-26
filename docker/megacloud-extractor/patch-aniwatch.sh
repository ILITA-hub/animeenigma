#!/bin/sh
# Patch the aniwatch npm package's extract5 method to use our megacloud extractor service
# instead of the broken GitHub key source.

ANIWATCH_JS="/app/node_modules/aniwatch/dist/index.js"
EXTRACTOR_URL="${MEGACLOUD_EXTRACTOR_URL:-http://megacloud-extractor:3200}"

if [ ! -f "$ANIWATCH_JS" ]; then
  echo "ERROR: $ANIWATCH_JS not found"
  exit 1
fi

echo "Patching aniwatch extract5 to use megacloud extractor at $EXTRACTOR_URL..."

# Replace the extract5 method body with one that calls our extractor service.
# We use node to do the patching since sed can't handle the complexity.
node -e "
const fs = require('fs');
const js = fs.readFileSync('$ANIWATCH_JS', 'utf8');

// Find and replace the extract5 method
const extract5Start = js.indexOf('async extract5(embedIframeURL)');
if (extract5Start === -1) {
  console.error('Could not find extract5 method');
  process.exit(1);
}

// Find the matching closing brace by counting braces
let braceCount = 0;
let bodyStart = js.indexOf('{', extract5Start);
let bodyEnd = bodyStart;
for (let i = bodyStart; i < js.length; i++) {
  if (js[i] === '{') braceCount++;
  if (js[i] === '}') braceCount--;
  if (braceCount === 0) {
    bodyEnd = i;
    break;
  }
}

const newBody = \`async extract5(embedIframeURL) {
    // Patched: use megacloud extractor service instead of broken key fetch
    const extractorUrl = '$EXTRACTOR_URL/extract?url=' + encodeURIComponent(embedIframeURL.href);
    console.log('Using megacloud extractor for:', embedIframeURL.href);
    const resp = await fetch(extractorUrl, { signal: AbortSignal.timeout(25000) });
    if (!resp.ok) throw new Error('Extractor returned ' + resp.status);
    const data = await resp.json();
    if (!data.sources || data.sources.length === 0) {
      throw new Error('Megacloud extractor returned no sources');
    }
    return {
      sources: data.sources.map(s => ({ url: s.url, isM3U8: s.isM3U8 !== false, type: s.type || 'hls' })),
      subtitles: (data.tracks || []).map(t => ({ url: t.url, lang: t.lang, default: t.default || false })),
      intro: data.intro || { start: 0, end: 0 },
      outro: data.outro || { start: 0, end: 0 },
    };
  }\`;

const patched = js.substring(0, extract5Start) + newBody + js.substring(bodyEnd + 1);
fs.writeFileSync('$ANIWATCH_JS', patched);
console.log('Successfully patched extract5 method');
"

echo "Patch complete. Starting aniwatch API..."
exec node /app/dist/src/server.js
