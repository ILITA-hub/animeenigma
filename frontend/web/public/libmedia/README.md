# libmedia (vendored)

Unmodified runtime of [libmedia](https://github.com/zhaohappy/libmedia) v1.3.1
(`@libmedia/avplayer`), LGPL-3.0-or-later — the lazy-loaded wasm "compat
engine" used by aePlayer to software-decode H.264 High 10 (Hi10P) streams in
browsers whose native decoders cannot (Firefox, Safari). Kept OUTSIDE the
Vite bundle on purpose:

- LGPL compliance: shipped as separate, unmodified, relinkable files.
- The webpack-built player spawns its own workers from sibling chunks —
  bundling would break their URLs.

`player/` = @libmedia/avplayer dist/esm (JS). `decode/`, `resample/`,
`stretchpitch/` = wasm modules fetched per-codec at runtime
(`wasmBaseUrl: '/libmedia'`). Upgrade = replace all of these together from
the same tagged release. Source: https://github.com/zhaohappy/libmedia
