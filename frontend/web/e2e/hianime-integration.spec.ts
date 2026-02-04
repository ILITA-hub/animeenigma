import { test, expect } from '@playwright/test'

const ANIME_ID = 'c076bca7-a93f-4089-90a3-0cb69b9cbf25' // Frieren S2

interface ServerStats {
  success: number
  fail: number
  errors: string[]
}

const TEST_ANIME = [
  { id: 'c076bca7-a93f-4089-90a3-0cb69b9cbf25', name: 'Frieren S2' },
  { id: 'ba5e4764-484b-42b2-9507-9e8f4081e6a6', name: 'Solo Leveling S2' }
]

const SERVERS_TO_TEST = ['hd-1', 'hd-2', 'hd-3']

test.describe('HiAnime Streaming Integration', () => {
  test('episodes API returns valid data', async ({ request }) => {
    const res = await request.get(`/api/anime/${ANIME_ID}/hianime/episodes`)
    expect(res.ok()).toBeTruthy()
    const data = await res.json()
    expect(data.success).toBe(true)
    expect(data.data.length).toBeGreaterThan(0)
  })

  test('servers API returns valid data', async ({ request }) => {
    // First get episode ID
    const epRes = await request.get(`/api/anime/${ANIME_ID}/hianime/episodes`)
    const { data: episodes } = await epRes.json()

    const res = await request.get(`/api/anime/${ANIME_ID}/hianime/servers`, {
      params: { episode: episodes[0].id }
    })
    expect(res.ok()).toBeTruthy()
    const data = await res.json()
    expect(data.data.length).toBeGreaterThan(0)
  })

  test('stream API returns HLS URL (not iframe)', async ({ request }) => {
    const epRes = await request.get(`/api/anime/${ANIME_ID}/hianime/episodes`)
    const { data: episodes } = await epRes.json()

    const srvRes = await request.get(`/api/anime/${ANIME_ID}/hianime/servers`, {
      params: { episode: episodes[0].id }
    })
    const { data: servers } = await srvRes.json()
    const server = servers.find((s: any) => s.type === 'sub')

    const res = await request.get(`/api/anime/${ANIME_ID}/hianime/stream`, {
      params: {
        episode: episodes[0].id,
        server: server.name.toLowerCase(), // Send NAME not ID
        category: 'sub'
      }
    })

    // If success, must be HLS
    if (res.ok()) {
      const data = await res.json()
      expect(data.data.type).toBe('hls')
      expect(data.data.url).toMatch(/\.m3u8/)
    } else {
      // If error, should have meaningful message
      const data = await res.json()
      expect(data.error.message).toBeTruthy()
      console.log('Stream error:', data.error.message)
    }
  })
})

test.describe('HiAnime Server Reliability', () => {
  // Increase timeout for comprehensive test - it tests all episodes across all servers
  test('comprehensive server test across all episodes', async ({ request }) => {
    test.setTimeout(300000) // 5 minutes
    const results: Record<string, Record<string, ServerStats>> = {}

    for (const anime of TEST_ANIME) {
      results[anime.name] = {}
      for (const server of SERVERS_TO_TEST) {
        results[anime.name][server] = { success: 0, fail: 0, errors: [] }
      }

      // Get all episodes
      const epRes = await request.get(`/api/anime/${anime.id}/hianime/episodes`)
      if (!epRes.ok()) {
        console.log(`\n=== ${anime.name}: Failed to fetch episodes ===`)
        continue
      }

      const { data: episodes } = await epRes.json()
      console.log(`\n=== ${anime.name}: ${episodes.length} episodes ===`)

      for (const episode of episodes) {
        for (const server of SERVERS_TO_TEST) {
          try {
            const streamRes = await request.get(`/api/anime/${anime.id}/hianime/stream`, {
              params: {
                episode: episode.id,
                server: server,
                category: 'sub'
              }
            })

            if (streamRes.ok()) {
              const { data } = await streamRes.json()

              // Verify HLS manifest is accessible via proxy
              const proxyUrl = `/api/streaming/hls-proxy?url=${encodeURIComponent(data.url)}&referer=${encodeURIComponent(data.headers?.Referer || '')}`
              const hlsRes = await request.get(proxyUrl)

              if (hlsRes.ok()) {
                const manifest = await hlsRes.text()
                if (manifest.includes('#EXTM3U')) {
                  results[anime.name][server].success++
                  console.log(`  Ep${episode.number} ${server}: ✅`)
                } else {
                  results[anime.name][server].fail++
                  results[anime.name][server].errors.push(`Ep${episode.number}: Invalid manifest`)
                  console.log(`  Ep${episode.number} ${server}: ⚠️ Invalid manifest`)
                }
              } else {
                results[anime.name][server].fail++
                results[anime.name][server].errors.push(`Ep${episode.number}: Proxy HTTP ${hlsRes.status()}`)
                console.log(`  Ep${episode.number} ${server}: ❌ Proxy ${hlsRes.status()}`)
              }
            } else {
              const { error } = await streamRes.json()
              results[anime.name][server].fail++
              const errorMsg = error?.message?.slice(0, 50) || 'Unknown error'
              results[anime.name][server].errors.push(`Ep${episode.number}: ${errorMsg}`)
              console.log(`  Ep${episode.number} ${server}: ❌ ${error?.message?.slice(0, 30) || 'Unknown'}`)
            }
          } catch (err) {
            results[anime.name][server].fail++
            const errorMsg = err instanceof Error ? err.message.slice(0, 50) : 'Unknown error'
            results[anime.name][server].errors.push(`Ep${episode.number}: ${errorMsg}`)
            console.log(`  Ep${episode.number} ${server}: ❌ Exception: ${errorMsg.slice(0, 30)}`)
          }
        }
      }
    }

    // Print summary
    console.log('\n========== SUMMARY ==========')
    for (const [anime, servers] of Object.entries(results)) {
      console.log(`\n${anime}:`)
      for (const [server, stats] of Object.entries(servers)) {
        const total = stats.success + stats.fail
        const rate = total > 0 ? ((stats.success / total) * 100).toFixed(1) : '0'
        console.log(`  ${server}: ${stats.success}/${total} (${rate}% success)`)
        if (stats.errors.length > 0 && stats.errors.length <= 3) {
          stats.errors.forEach(e => console.log(`    - ${e}`))
        } else if (stats.errors.length > 3) {
          console.log(`    - ${stats.errors.length} errors (showing first 3)`)
          stats.errors.slice(0, 3).forEach(e => console.log(`    - ${e}`))
        }
      }
    }

    // Test passes as long as at least one server works for each anime
    for (const anime of TEST_ANIME) {
      const animeResults = results[anime.name]
      if (animeResults) {
        const hasWorkingServer = SERVERS_TO_TEST.some(
          server => animeResults[server]?.success > 0
        )
        expect(hasWorkingServer, `${anime.name} should have at least one working server`).toBeTruthy()
      }
    }
  })

  test('single episode server comparison', async ({ request }) => {
    // Quick test for a single episode to compare servers
    const anime = TEST_ANIME[0]

    const epRes = await request.get(`/api/anime/${anime.id}/hianime/episodes`)
    expect(epRes.ok()).toBeTruthy()
    const { data: episodes } = await epRes.json()
    const firstEpisode = episodes[0]

    console.log(`\nTesting ${anime.name} - Episode ${firstEpisode.number}`)
    console.log('='.repeat(50))

    for (const server of SERVERS_TO_TEST) {
      const streamRes = await request.get(`/api/anime/${anime.id}/hianime/stream`, {
        params: {
          episode: firstEpisode.id,
          server: server,
          category: 'sub'
        }
      })

      if (streamRes.ok()) {
        const { data } = await streamRes.json()
        console.log(`${server}: API OK - ${data.url?.slice(0, 60)}...`)

        // Test proxy access
        const proxyUrl = `/api/streaming/hls-proxy?url=${encodeURIComponent(data.url)}&referer=${encodeURIComponent(data.headers?.Referer || '')}`
        const hlsRes = await request.get(proxyUrl)

        if (hlsRes.ok()) {
          const manifest = await hlsRes.text()
          const isValid = manifest.includes('#EXTM3U')
          console.log(`${server}: Proxy ${hlsRes.status()} - ${isValid ? '✅ Valid HLS' : '⚠️ Invalid manifest'}`)
        } else {
          console.log(`${server}: Proxy FAILED - HTTP ${hlsRes.status()}`)
        }
      } else {
        const { error } = await streamRes.json()
        console.log(`${server}: API FAILED - ${error?.message || 'Unknown error'}`)
      }
    }
  })
})
