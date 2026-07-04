import { describe, it, expect } from 'vitest'
import { parseStoryboardVtt, cueAt } from './storyboardVtt'

const BASE = 'https://animeenigma.org/api/streaming/hls-proxy?url=x%2Fstoryboard.vtt'

const SAMPLE = `WEBVTT

00:00:00.000 --> 00:00:05.000
/api/streaming/hls-proxy?url=a&exp=1&sig=b#xywh=0,0,160,90

00:00:05.000 --> 00:00:10.000
/api/streaming/hls-proxy?url=a&exp=1&sig=b#xywh=160,0,160,90

01:00:00.000 --> 01:00:05.000
storyboard_002.jpg#xywh=320,90,160,90
`

describe('parseStoryboardVtt', () => {
  it('parses proxied cues with times, url, and xywh', () => {
    const cues = parseStoryboardVtt(SAMPLE, BASE)
    expect(cues).toHaveLength(2)
    expect(cues[0]).toMatchObject({ start: 0, end: 5, x: 0, y: 0, w: 160, h: 90 })
    expect(cues[1].x).toBe(160)
    expect(cues[0].url).toBe('https://animeenigma.org/api/streaming/hls-proxy?url=a&exp=1&sig=b')
  })
  it('SKIPS bare-relative payloads — the backend proxy rewriting cues to signed absolute URLs is the contract; a client-resolved relative sheet URL could never carry url=/sig= and would always 404', () => {
    const cues = parseStoryboardVtt(SAMPLE, BASE)
    expect(cues.every((c) => c.start < 3600)).toBe(true)
  })
  it('skips malformed cues instead of throwing', () => {
    expect(parseStoryboardVtt('WEBVTT\n\ngarbage\nnot-a-timing\n', BASE)).toEqual([])
    expect(parseStoryboardVtt('', BASE)).toEqual([])
  })
  it('anchors a ROOT-RELATIVE base on the document origin instead of throwing — this is the DEFAULT shape when VITE_HLS_PROXY_BASE is unset, so every cue must still resolve, not silently empty out', () => {
    const rootRelativeBase = '/api/streaming/hls-proxy?url=x%2Fstoryboard.vtt'
    const cues = parseStoryboardVtt(SAMPLE, rootRelativeBase)
    expect(cues).toHaveLength(2)
    const origin = new URL(window.location.href).origin
    expect(cues[0].url).toBe(`${origin}/api/streaming/hls-proxy?url=a&exp=1&sig=b`)
    expect(cues[1].url).toBe(`${origin}/api/streaming/hls-proxy?url=a&exp=1&sig=b`)
  })
})

describe('cueAt', () => {
  const cues = parseStoryboardVtt(SAMPLE, BASE)
  it('finds the covering cue and handles boundaries [start, end)', () => {
    expect(cueAt(cues, 0)?.x).toBe(0)
    expect(cueAt(cues, 4.9)?.x).toBe(0)
    expect(cueAt(cues, 5)?.x).toBe(160)
  })
  it('returns null outside all cues and clamps nothing', () => {
    expect(cueAt(cues, 20)).toBeNull()
    expect(cueAt([], 1)).toBeNull()
  })
})
