// Brainstorming visual-companion server (recreated 2026-06-10, gacha banner UX).
// Contract per feedback_design_artifact_workflow: port 58363, serves the NEWEST
// .html file in ./content/ at "/", 2h idle self-shutdown. User tunnel:
//   ssh -L 3000:localhost:58363 <server>   →  open http://localhost:3000
const http = require('http')
const fs = require('fs')
const path = require('path')

const PORT = 58363
const CONTENT = path.join(__dirname, 'content')
const IDLE_MS = 2 * 60 * 60 * 1000
let lastHit = Date.now()

function newestHtml() {
  const files = fs.readdirSync(CONTENT).filter(f => f.endsWith('.html'))
  files.sort((a, b) => fs.statSync(path.join(CONTENT, b)).mtimeMs - fs.statSync(path.join(CONTENT, a)).mtimeMs)
  return files[0] || null
}

const srv = http.createServer((req, res) => {
  lastHit = Date.now()
  const f = newestHtml()
  if (!f) { res.writeHead(404); return res.end('no artifact') }
  res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8', 'Cache-Control': 'no-store' })
  res.end(fs.readFileSync(path.join(CONTENT, f)))
})
srv.listen(PORT, '127.0.0.1', () => console.log(`brainstorm server on :${PORT}, serving newest of ${CONTENT}`))
setInterval(() => { if (Date.now() - lastHit > IDLE_MS) { console.log('idle 2h, bye'); process.exit(0) } }, 60_000)
