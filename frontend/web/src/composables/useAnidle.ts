/**
 * useAnidle — composable for the anidle anime-guessing game.
 *
 * Single interface for both guest and logged-in users. Guest state is
 * persisted to localStorage keyed by date; auth state is hydrated from the
 * backend on mount and on auth transitions.
 *
 * NO-CHEAT RULE: `dailyAnswer` and `endlessAnswer` are only ever set from
 * a backend response — never computed locally. The composable enforces this.
 */
import { ref, computed, watch, onMounted, type Ref, type ComputedRef } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { anidleApi, buildShareText } from '@/api/anidle'
import type { VisibleAnime, GuessOutcome, UserStats, LeaderEntry } from '@/api/anidle'

// ─── localStorage keys ─────────────────────────────────────────────────────

function lsKey(date: string) { return `anidle:daily:${date}` }
function lsSolvedKey(date: string) { return `anidle:daily:solved:${date}` }
function lsGaveUpKey(date: string) { return `anidle:daily:gaveup:${date}` }
function lsAnswerKey(date: string) { return `anidle:daily:answer:${date}` }
function todayISO(): string {
  return new Date().toISOString().slice(0, 10)
}

// ─── Return type ───────────────────────────────────────────────────────────

export interface UseAnidleReturn {
  mode: Ref<'daily' | 'endless'>
  dailyDate: Ref<string>
  dailyGuesses: Ref<GuessOutcome[]>
  dailySolved: Ref<boolean>
  dailyGaveUp: Ref<boolean>
  dailyAnswer: Ref<VisibleAnime | null>
  dailyAttempts: ComputedRef<number>

  endlessToken: Ref<string | null>
  endlessGuesses: Ref<GuessOutcome[]>
  endlessSolved: Ref<boolean>
  endlessAnswer: Ref<VisibleAnime | null>

  isLoading: Ref<boolean>
  isGuessing: Ref<boolean>
  error: Ref<string | null>

  stats: Ref<UserStats | null>
  leaderboard: Ref<LeaderEntry[]>

  submitDailyGuess(animeId: string): Promise<void>
  submitGiveUp(): Promise<void>
  startEndless(): Promise<void>
  submitEndlessGuess(animeId: string): Promise<void>
  setMode(m: 'daily' | 'endless'): void
  fetchStats(): Promise<void>
  fetchLeaderboard(date: string): Promise<void>
  shareResult(): string
}

// ─── Composable ────────────────────────────────────────────────────────────

export function useAnidle(): UseAnidleReturn {
  const auth = useAuthStore()

  const mode = ref<'daily' | 'endless'>('daily')
  const dailyDate = ref<string>(todayISO())
  const dailyGuesses = ref<GuessOutcome[]>([])
  const dailySolved = ref(false)
  const dailyGaveUp = ref(false)
  const dailyAnswer = ref<VisibleAnime | null>(null)

  const endlessToken = ref<string | null>(null)
  const endlessGuesses = ref<GuessOutcome[]>([])
  const endlessSolved = ref(false)
  const endlessAnswer = ref<VisibleAnime | null>(null)

  const isLoading = ref(false)
  const isGuessing = ref(false)
  const error = ref<string | null>(null)

  const stats = ref<UserStats | null>(null)
  const leaderboard = ref<LeaderEntry[]>([])

  const dailyAttempts = computed(() => dailyGuesses.value.length)

  // ── localStorage helpers ──────────────────────────────────────────────────

  function saveGuestState() {
    const date = dailyDate.value
    try {
      localStorage.setItem(lsKey(date), JSON.stringify(dailyGuesses.value))
      if (dailySolved.value) localStorage.setItem(lsSolvedKey(date), '1')
      if (dailyGaveUp.value) localStorage.setItem(lsGaveUpKey(date), '1')
      if (dailyAnswer.value) {
        localStorage.setItem(lsAnswerKey(date), JSON.stringify(dailyAnswer.value))
      }
    } catch {
      // localStorage may be unavailable
    }
  }

  function loadGuestState() {
    const date = todayISO()
    dailyDate.value = date
    try {
      const raw = localStorage.getItem(lsKey(date))
      if (raw) dailyGuesses.value = JSON.parse(raw) as GuessOutcome[]
      dailySolved.value = localStorage.getItem(lsSolvedKey(date)) === '1'
      dailyGaveUp.value = localStorage.getItem(lsGaveUpKey(date)) === '1'
      const rawAnswer = localStorage.getItem(lsAnswerKey(date))
      if (rawAnswer) dailyAnswer.value = JSON.parse(rawAnswer) as VisibleAnime
    } catch {
      // corrupt storage — start fresh
      dailyGuesses.value = []
      dailySolved.value = false
      dailyGaveUp.value = false
      dailyAnswer.value = null
    }
  }

  // ── Server state load ─────────────────────────────────────────────────────

  async function loadServerState() {
    isLoading.value = true
    error.value = null
    try {
      const res = await anidleApi.getDailyState()
      const state = (res.data?.data ?? res.data) as import('@/api/anidle').DailyState
      dailyDate.value = state.date ?? todayISO()
      dailyGuesses.value = state.guesses ?? []
      dailySolved.value = state.solved ?? false
      dailyGaveUp.value = state.gave_up ?? false
      if (state.answer) dailyAnswer.value = state.answer
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to load game state'
    } finally {
      isLoading.value = false
    }
  }

  // ── Mount ─────────────────────────────────────────────────────────────────

  onMounted(async () => {
    if (auth.isAuthenticated) {
      await loadServerState()
    } else {
      loadGuestState()
    }
  })

  // Re-fetch when auth state changes (login / logout)
  watch(
    () => auth.token,
    (newToken, oldToken) => {
      if (newToken !== oldToken && oldToken !== undefined) {
        if (newToken) {
          void loadServerState()
        } else {
          loadGuestState()
        }
      }
    },
  )

  // ── Daily actions ─────────────────────────────────────────────────────────

  async function submitDailyGuess(animeId: string): Promise<void> {
    if (dailySolved.value || dailyGaveUp.value || isGuessing.value) return
    isGuessing.value = true
    error.value = null
    try {
      const res = await anidleApi.dailyGuess(animeId)
      const outcome = (res.data?.data ?? res.data) as GuessOutcome
      dailyGuesses.value = [...dailyGuesses.value, outcome]
      if (outcome.solved) {
        dailySolved.value = true
        // answer is present only on solve — no-cheat: only set from server
        if (outcome.answer) dailyAnswer.value = outcome.answer
      }
      if (!auth.isAuthenticated) saveGuestState()
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to submit guess'
    } finally {
      isGuessing.value = false
    }
  }

  async function submitGiveUp(): Promise<void> {
    if (dailySolved.value || dailyGaveUp.value || isGuessing.value) return
    isGuessing.value = true
    error.value = null
    try {
      const res = await anidleApi.dailyGiveUp()
      const body = (res.data?.data ?? res.data) as VisibleAnime | { answer?: VisibleAnime }
      // tolerate both shapes: data = VisibleAnime, or data = { answer: VisibleAnime }
      const revealed = ((body as { answer?: VisibleAnime })?.answer ?? body) as VisibleAnime
      dailyGaveUp.value = true
      // answer comes only from server — no-cheat enforced
      dailyAnswer.value = revealed
      if (!auth.isAuthenticated) saveGuestState()
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to give up'
    } finally {
      isGuessing.value = false
    }
  }

  // ── Endless actions ───────────────────────────────────────────────────────

  async function startEndless(): Promise<void> {
    isLoading.value = true
    error.value = null
    try {
      const res = await anidleApi.endlessNew()
      const data = res.data?.data ?? res.data
      endlessToken.value = (data as { round_token: string }).round_token
      endlessGuesses.value = []
      endlessSolved.value = false
      endlessAnswer.value = null
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to start endless round'
    } finally {
      isLoading.value = false
    }
  }

  async function submitEndlessGuess(animeId: string): Promise<void> {
    if (!endlessToken.value || endlessSolved.value || isGuessing.value) return
    isGuessing.value = true
    error.value = null
    try {
      const res = await anidleApi.endlessGuess(endlessToken.value, animeId)
      const outcome = (res.data?.data ?? res.data) as GuessOutcome
      endlessGuesses.value = [...endlessGuesses.value, outcome]
      if (outcome.solved) {
        endlessSolved.value = true
        if (outcome.answer) endlessAnswer.value = outcome.answer
      }
    } catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to submit guess'
    } finally {
      isGuessing.value = false
    }
  }

  // ── Stats / leaderboard ───────────────────────────────────────────────────

  async function fetchStats(): Promise<void> {
    if (!auth.isAuthenticated) return
    try {
      const res = await anidleApi.getStats()
      if (res.status === 204) return
      const data = res.data?.data ?? res.data
      if (data) stats.value = data as UserStats
    } catch {
      // non-critical — silently ignore
    }
  }

  async function fetchLeaderboard(date: string): Promise<void> {
    try {
      const res = await anidleApi.getLeaderboard(date)
      const data = res.data?.data ?? res.data
      leaderboard.value = (data as LeaderEntry[]) ?? []
    } catch {
      leaderboard.value = []
    }
  }

  // ── Mode ──────────────────────────────────────────────────────────────────

  function setMode(m: 'daily' | 'endless') {
    mode.value = m
  }

  // ── Share ─────────────────────────────────────────────────────────────────

  function shareResult(): string {
    return buildShareText(dailyGuesses.value, dailyDate.value, dailySolved.value)
  }

  return {
    mode,
    dailyDate,
    dailyGuesses,
    dailySolved,
    dailyGaveUp,
    dailyAnswer,
    dailyAttempts,

    endlessToken,
    endlessGuesses,
    endlessSolved,
    endlessAnswer,

    isLoading,
    isGuessing,
    error,

    stats,
    leaderboard,

    submitDailyGuess,
    submitGiveUp,
    startEndless,
    submitEndlessGuess,
    setMode,
    fetchStats,
    fetchLeaderboard,
    shareResult,
  }
}
