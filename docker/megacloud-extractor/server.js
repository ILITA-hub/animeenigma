const http = require("http");
const url = require("url");
const https = require("https");

const PORT = process.env.PORT || 3200;

// Simple HTTPS fetch that mimics curl (no browser headers)
function fetchUrl(targetUrl, headers = {}) {
  return new Promise((resolve, reject) => {
    const parsed = new URL(targetUrl);
    const options = {
      hostname: parsed.hostname,
      path: parsed.pathname + parsed.search,
      method: "GET",
      headers: {
        "User-Agent":
          "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
        Accept: "*/*",
        ...headers,
      },
    };

    const req = https.request(options, (res) => {
      let data = "";
      res.on("data", (chunk) => (data += chunk));
      res.on("end", () => resolve({ status: res.statusCode, data, headers: res.headers }));
    });
    req.on("error", reject);
    req.setTimeout(15000, () => {
      req.destroy(new Error("Request timeout"));
    });
    req.end();
  });
}

async function extractSources(embedUrl) {
  const result = {
    sources: [],
    tracks: [],
    intro: { start: 0, end: 0 },
    outro: { start: 0, end: 0 },
  };

  // Step 1: Fetch embed page HTML (like curl, no browser headers)
  console.log(`Step 1: Fetching embed page...`);
  const pageResp = await fetchUrl(embedUrl, {
    Referer: "https://aniwatchtv.to/",
  });

  if (pageResp.data.includes("404-error")) {
    throw new Error("Video not found on megacloud (404 page)");
  }

  // Step 2: Extract client key from HTML
  const patterns = [
    { regex: /<meta name="_gg_fb" content="([a-zA-Z0-9]+)">/, name: "_gg_fb" },
    { regex: /<!--\s+_is_th:([0-9a-zA-Z]+)\s+-->/, name: "_is_th" },
    { regex: /data-dpi="([0-9a-zA-Z]+)"/, name: "data-dpi" },
    { regex: /window\._xy_ws\s*=\s*['"`]([0-9a-zA-Z]+)['"`]/, name: "_xy_ws" },
  ];

  let clientKey = null;
  for (const p of patterns) {
    const match = pageResp.data.match(p.regex);
    if (match) {
      clientKey = match[1];
      console.log(`Step 2: Found client key via ${p.name}: ${clientKey.substring(0, 20)}...`);
      break;
    }
  }

  if (!clientKey) {
    console.log(`Page content (first 500 chars): ${pageResp.data.substring(0, 500)}`);
    console.log(`Status: ${pageResp.status}`);
    throw new Error("Could not extract client key from embed page");
  }

  // Extract video ID from embed URL
  const videoIdMatch = embedUrl.match(/\/([^\/\?]+)\?/);
  const videoId = videoIdMatch ? videoIdMatch[1] : null;
  if (!videoId) {
    throw new Error("Could not extract video ID from URL");
  }

  // Step 3: Call getSources API
  const sourcesUrl = `https://megacloud.blog/embed-2/v3/e-1/getSources?id=${videoId}&_k=${clientKey}`;
  console.log(`Step 3: Calling getSources...`);
  const sourcesResp = await fetchUrl(sourcesUrl, {
    Referer: embedUrl,
    "X-Requested-With": "XMLHttpRequest",
  });

  let sourcesData;
  try {
    sourcesData = JSON.parse(sourcesResp.data);
  } catch {
    throw new Error(`Failed to parse getSources response: ${sourcesResp.data.substring(0, 200)}`);
  }

  if (sourcesData.error) {
    console.log(`getSources error: ${sourcesData.error}`);
    throw new Error(`getSources: ${sourcesData.error}`);
  }

  console.log(`Step 3: Got sources response (encrypted: ${!!sourcesData.encrypted}, status: ${sourcesResp.status})`);

  // Extract tracks (subtitles)
  if (sourcesData.tracks) {
    result.tracks = sourcesData.tracks
      .filter((t) => t.kind === "captions")
      .map((t) => ({
        url: t.file,
        lang: t.label || t.kind,
        default: t.default || false,
      }));
  }

  // Extract intro/outro
  if (sourcesData.intro) result.intro = sourcesData.intro;
  if (sourcesData.outro) result.outro = sourcesData.outro;

  // Step 4: Handle sources
  if (!sourcesData.encrypted && Array.isArray(sourcesData.sources)) {
    // Unencrypted sources - direct HLS URLs
    result.sources = sourcesData.sources.map((s) => ({
      url: s.file,
      type: s.type || "hls",
      isM3U8: true,
    }));
  } else if (typeof sourcesData.sources === "string") {
    // Encrypted sources - need decryption
    // Try to fetch the decryption script and extract variables
    console.log(`Step 4: Sources are encrypted, attempting decryption...`);
    try {
      const decrypted = await decryptSources(sourcesData.sources, embedUrl);
      if (decrypted) {
        result.sources = decrypted.map((s) => ({
          url: s.file,
          type: s.type || "hls",
          isM3U8: true,
        }));
      }
    } catch (e) {
      console.log(`Decryption failed: ${e.message}`);
      // Return what we have (tracks, intro/outro) without sources
    }
  }

  return result;
}

// Decrypt encrypted sources using the cinemaxhq keys approach
async function decryptSources(encryptedString, referer) {
  const crypto = require("crypto");

  // Fetch the decryption key offsets
  const keyResp = await fetchUrl(
    "https://raw.githubusercontent.com/cinemaxhq/keys/e1/key"
  );
  let keyData;
  try {
    keyData = JSON.parse(keyResp.data);
  } catch {
    throw new Error("Failed to parse decryption key");
  }

  // Extract secret and encrypted source using key offsets
  let secret = "";
  const encArray = encryptedString.split("");
  let currentIndex = 0;

  for (const [start, length] of keyData) {
    const absStart = start + currentIndex;
    const absEnd = absStart + length;
    for (let i = absStart; i < absEnd; i++) {
      secret += encryptedString[i];
      encArray[i] = "";
    }
    currentIndex += length;
  }

  const encryptedSource = encArray.join("");

  // AES-256-CBC decryption (OpenSSL compatible)
  const cypher = Buffer.from(encryptedSource, "base64");
  const salt = cypher.subarray(8, 16);
  const password = Buffer.concat([Buffer.from(secret, "binary"), salt]);

  const md5Hashes = [];
  let digest = password;
  for (let i = 0; i < 3; i++) {
    md5Hashes[i] = crypto.createHash("md5").update(digest).digest();
    digest = Buffer.concat([md5Hashes[i], password]);
  }

  const key = Buffer.concat([md5Hashes[0], md5Hashes[1]]);
  const iv = md5Hashes[2];
  const contents = cypher.subarray(16);

  const decipher = crypto.createDecipheriv("aes-256-cbc", key, iv);
  const decrypted =
    decipher.update(contents, undefined, "utf8") + decipher.final("utf8");

  return JSON.parse(decrypted);
}

const server = http.createServer(async (req, res) => {
  const parsed = url.parse(req.url, true);

  if (parsed.pathname === "/health") {
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ status: "ok" }));
    return;
  }

  if (parsed.pathname === "/extract") {
    const embedUrl = parsed.query.url;
    if (!embedUrl) {
      res.writeHead(400, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ error: "url parameter required" }));
      return;
    }

    try {
      console.log(`\nExtracting: ${embedUrl}`);
      const start = Date.now();
      const result = await extractSources(embedUrl);
      const elapsed = Date.now() - start;
      console.log(
        `Result: ${result.sources.length} sources, ${result.tracks.length} tracks in ${elapsed}ms`
      );
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify(result));
    } catch (err) {
      console.error(`Extraction failed: ${err.message}`);
      res.writeHead(500, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ error: err.message }));
    }
    return;
  }

  res.writeHead(404, { "Content-Type": "application/json" });
  res.end(JSON.stringify({ error: "not found" }));
});

server.listen(PORT, () => {
  console.log(`Megacloud extractor listening on port ${PORT}`);
});
