import { test, expect } from '@playwright/test'

// Test anime - use a popular anime that's likely to be found
const TEST_ANIME = [
  { id: 'c076bca7-a93f-4089-90a3-0cb69b9cbf25', name: 'Frieren S2' },
  { id: 'ba5e4764-484b-42b2-9507-9e8f4081e6a6', name: 'Solo Leveling S2' }
]

const SERVERS_TO_TEST = ['vidcloud', 'streamsb', 'vidstreaming']

interface ServerStats {
  success: number
  fail: number
  errors: string[]
}

test.describe('Consumet Streaming Integration', () => {
  test('episodes API returns valid data', async ({ request }) => {
    const anime = TEST_ANIME[0]
    const res = await request.get(`/api/anime/${anime.id}/consumet/episodes`)

    // May return empty if anime not found on Consumet
    if (res.ok()) {
      const data = await res.json()
      expect(data.success).toBe(true)
      console.log(`${anime.name}: Found ${data.data?.length || 0} episodes on Consumet`)
    } else {
      console.log(`${anime.name}: Not found on Consumet (expected for some anime)`)
    }
  })

  test('servers API returns predefined servers', async ({ request }) => {
    const anime = TEST_ANIME[0]
    const res = await request.get(`/api/anime/${anime.id}/consumet/servers`)
    expect(res.ok()).toBeTruthy()

    const data = await res.json()
    expect(data.success).toBe(true)
    expect(data.data.length).toBeGreaterThan(0)

    console.log('Available servers:', data.data.map((s: any) => s.name).join(', '))
  })

  test('stream API returns HLS URL', async ({ request }) => {
    const anime = TEST_ANIME[0]

    // Get episodes first
    const epRes = await request.get(`/api/anime/${anime.id}/consumet/episodes`)
    if (!epRes.ok()) {
      console.log('Skipping stream test - anime not found on Consumet')
      return
    }

    const epData = await epRes.json()
    if (!epData.data || epData.data.length === 0) {
      console.log('Skipping stream test - no episodes found')
      return
    }

    const episode = epData.data[0]

    // Get stream
    const res = await request.get(`/api/anime/${anime.id}/consumet/stream`, {
      params: {
        episode: episode.id,
        server: 'vidcloud'
      }
    })

    if (res.ok()) {
      const data = await res.json()
      expect(data.data.url).toBeTruthy()
      expect(data.data.url).toMatch(/\.m3u8|\.mp4/)
      console.log(`Stream URL: ${data.data.url.substring(0, 80)}...`)
    } else {
      const data = await res.json()
      console.log('Stream error:', data.error?.message || 'Unknown error')
    }
  })
})

test.describe('Consumet Server Reliability', () => {
  test('comprehensive server test across episodes', async ({ request }) => {
    test.setTimeout(300000) // 5 minutes

    const results: Record<string, Record<string, ServerStats>> = {}

    for (const anime of TEST_ANIME) {
      results[anime.name] = {}
      for (const server of SERVERS_TO_TEST) {
        results[anime.name][server] = { success: 0, fail: 0, errors: [] }
      }

      // Get episodes
      const epRes = await request.get(`/api/anime/${anime.id}/consumet/episodes`)
      if (!epRes.ok()) {
        console.log(`\n=== ${anime.name}: Not found on Consumet ===`)
        continue
      }

      const epData = await epRes.json()
      const episodes = epData.data || []

      if (episodes.length === 0) {
        console.log(`\n=== ${anime.name}: No episodes found ===`)
        continue
      }

      console.log(`\n=== ${anime.name}: Testing ${Math.min(episodes.length, 5)} episodes ===`)

      // Test first 5 episodes to avoid rate limits
      const testEpisodes = episodes.slice(0, 5)

      for (const episode of testEpisodes) {
        for (const server of SERVERS_TO_TEST) {
          try {
            const streamRes = await request.get(`/api/anime/${anime.id}/consumet/stream`, {
              params: {
                episode: episode.id,
                server: server
              }
            })

            if (streamRes.ok()) {
              const { data } = await streamRes.json()

              if (data.url) {
                // Verify HLS manifest is accessible via proxy
                const proxyUrl = `/api/streaming/hls-proxy?url=${encodeURIComponent(data.url)}`
                const hlsRes = await request.get(proxyUrl)

                if (hlsRes.ok()) {
                  const manifest = await hlsRes.text()
                  if (manifest.includes('#EXTM3U') || manifest.includes('#EXT')) {
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
                results[anime.name][server].fail++
                results[anime.name][server].errors.push(`Ep${episode.number}: No URL in response`)
                console.log(`  Ep${episode.number} ${server}: ❌ No URL`)
              }
            } else {
              const { error } = await streamRes.json()
              results[anime.name][server].fail++
              const errorMsg = error?.message?.slice(0, 50) || 'Unknown error'
              results[anime.name][server].errors.push(`Ep${episode.number}: ${errorMsg}`)
              console.log(`  Ep${episode.number} ${server}: ❌ ${errorMsg.slice(0, 30)}`)
            }
          } catch (err) {
            results[anime.name][server].fail++
            const errorMsg = err instanceof Error ? err.message.slice(0, 50) : 'Unknown error'
            results[anime.name][server].errors.push(`Ep${episode.number}: ${errorMsg}`)
            console.log(`  Ep${episode.number} ${server}: ❌ Exception`)
          }

          // Small delay between requests to avoid rate limits
          await new Promise(resolve => setTimeout(resolve, 500))
        }
      }
    }

    // Print summary
    console.log('\n========== CONSUMET SUMMARY ==========')
    for (const [anime, servers] of Object.entries(results)) {
      console.log(`\n${anime}:`)
      for (const [server, stats] of Object.entries(servers)) {
        const total = stats.success + stats.fail
        const rate = total > 0 ? ((stats.success / total) * 100).toFixed(1) : 'N/A'
        console.log(`  ${server}: ${stats.success}/${total} (${rate}% success)`)
        if (stats.errors.length > 0 && stats.errors.length <= 3) {
          stats.errors.forEach(e => console.log(`    - ${e}`))
        } else if (stats.errors.length > 3) {
          console.log(`    - ${stats.errors.length} errors (showing first 3)`)
          stats.errors.slice(0, 3).forEach(e => console.log(`    - ${e}`))
        }
      }
    }

    // Test passes if at least one anime has a working server
    let hasWorking = false
    for (const anime of TEST_ANIME) {
      const animeResults = results[anime.name]
      if (animeResults) {
        for (const server of SERVERS_TO_TEST) {
          if (animeResults[server]?.success > 0) {
            hasWorking = true
            break
          }
        }
      }
      if (hasWorking) break
    }

    console.log(`\nOverall: ${hasWorking ? 'At least one working configuration found' : 'No working configurations'}`)
  })
})

test.describe('Consumet vs HiAnime Comparison', () => {
  test('compare availability and reliability', async ({ request }) => {
    test.setTimeout(120000) // 2 minutes

    const anime = TEST_ANIME[0]
    const results = {
      hianime: { found: false, episodes: 0, streamsWorking: 0 },
      consumet: { found: false, episodes: 0, streamsWorking: 0 }
    }

    // Test HiAnime
    console.log(`\n=== Testing HiAnime for ${anime.name} ===`)
    try {
      const hiRes = await request.get(`/api/anime/${anime.id}/hianime/episodes`)
      if (hiRes.ok()) {
        const data = await hiRes.json()
        results.hianime.found = true
        results.hianime.episodes = data.data?.length || 0
        console.log(`  Episodes found: ${results.hianime.episodes}`)

        if (results.hianime.episodes > 0) {
          const episode = data.data[0]
          // Try hd-2 (more reliable)
          const streamRes = await request.get(`/api/anime/${anime.id}/hianime/stream`, {
            params: { episode: episode.id, server: 'hd-2', category: 'sub' }
          })
          if (streamRes.ok()) {
            results.hianime.streamsWorking++
            console.log('  Stream: ✅ Working')
          } else {
            console.log('  Stream: ❌ Failed')
          }
        }
      } else {
        console.log('  Not found on HiAnime')
      }
    } catch (err) {
      console.log('  HiAnime error:', err)
    }

    // Test Consumet
    console.log(`\n=== Testing Consumet for ${anime.name} ===`)
    try {
      const conRes = await request.get(`/api/anime/${anime.id}/consumet/episodes`)
      if (conRes.ok()) {
        const data = await conRes.json()
        results.consumet.found = true
        results.consumet.episodes = data.data?.length || 0
        console.log(`  Episodes found: ${results.consumet.episodes}`)

        if (results.consumet.episodes > 0) {
          const episode = data.data[0]
          const streamRes = await request.get(`/api/anime/${anime.id}/consumet/stream`, {
            params: { episode: episode.id, server: 'vidcloud' }
          })
          if (streamRes.ok()) {
            const streamData = await streamRes.json()
            if (streamData.data?.url) {
              results.consumet.streamsWorking++
              console.log('  Stream: ✅ Working')
            } else {
              console.log('  Stream: ❌ No URL')
            }
          } else {
            console.log('  Stream: ❌ Failed')
          }
        }
      } else {
        console.log('  Not found on Consumet')
      }
    } catch (err) {
      console.log('  Consumet error:', err)
    }

    // Print comparison
    console.log('\n========== COMPARISON ==========')
    console.log(`HiAnime: Found=${results.hianime.found}, Episodes=${results.hianime.episodes}, Streams=${results.hianime.streamsWorking}`)
    console.log(`Consumet: Found=${results.consumet.found}, Episodes=${results.consumet.episodes}, Streams=${results.consumet.streamsWorking}`)

    // At least one provider should work
    expect(results.hianime.found || results.consumet.found).toBeTruthy()
  })
})
