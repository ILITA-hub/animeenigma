import { ref, watch, type Ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import type { Anime } from '@/composables/useAnime'
import { useConfirm } from '@/composables/useConfirm'
import { commentApi } from '@/api/client'
import { runeLen } from '@/composables/anime/animeFormatters'
import { UGC_ALLOWED, type UgcTab, type Comment, type ApiError } from './types'

/**
 * Comments + UGC tab state for the anime page (Plan 1-6, SOCIAL-06 — extracted
 * from Anime.vue): the reviews/comments tab (URL-persisted via ?ugc=), the
 * paginated comments feed, and the post/edit/delete flows with optimistic
 * updates and cursor-preserving mutations.
 */
export function useAnimeComments(anime: Ref<Anime | null>) {
  const route = useRoute()
  const router = useRouter()
  const { t } = useI18n()
  const { confirm } = useConfirm()

  // UGC tab state, URL-persisted via ?ugc=reviews|comments via two watchers.
  // Default = 'reviews'. Unknown values fall back to 'reviews' synchronously
  // (initial state set BEFORE first render, no flicker on deep links).
  const _initialUgc = (route.query.ugc as string | undefined)
  const ugcTab = ref<UgcTab>(
    _initialUgc && (UGC_ALLOWED as readonly string[]).includes(_initialUgc)
      ? (_initialUgc as UgcTab)
      : 'reviews'
  )
  const comments = ref<Comment[]>([])
  const commentsHasMore = ref(false)
  const commentsNextCursor = ref<string>('')
  const commentsLoading = ref(false)
  const commentsLoadingMore = ref(false)
  const commentsError = ref('')
  const commentsFetched = ref(false)

  const newCommentBody = ref('')
  const posting = ref(false)
  const postError = ref('')

  const editingCommentId = ref<string | null>(null)
  const editingBody = ref('')
  const editError = ref('')
  const editSaving = ref(false)
  const deleteError = ref('')

  const fetchComments = async () => {
    if (!anime.value) return
    commentsLoading.value = true
    commentsError.value = ''
    try {
      const resp = await commentApi.getAnimeComments(anime.value.id, { limit: 50 })
      const data = resp.data?.data || resp.data || {}
      comments.value = data.comments || []
      commentsHasMore.value = !!data.has_more
      commentsNextCursor.value = data.next_cursor || ''
      commentsFetched.value = true
    } catch (err) {
      console.error('Failed to fetch comments:', err)
      commentsError.value = t('anime.ugc.loadFailed')
    } finally {
      commentsLoading.value = false
    }
  }

  const loadMoreComments = async () => {
    if (!anime.value || !commentsHasMore.value || commentsLoadingMore.value) return
    commentsLoadingMore.value = true
    commentsError.value = ''
    try {
      const resp = await commentApi.getAnimeComments(anime.value.id, {
        limit: 50,
        cursor: commentsNextCursor.value,
      })
      const data = resp.data?.data || resp.data || {}
      const newPage: Comment[] = data.comments || []
      // Dedup by id in case the cursor reaches an already-loaded boundary.
      const known = new Set(comments.value.map((c) => c.id))
      for (const c of newPage) {
        if (!known.has(c.id)) comments.value.push(c)
      }
      commentsHasMore.value = !!data.has_more
      commentsNextCursor.value = data.next_cursor || ''
    } catch (err) {
      console.error('Failed to load more comments:', err)
      commentsError.value = t('anime.ugc.loadMoreFailed')
    } finally {
      commentsLoadingMore.value = false
    }
  }

  const postComment = async () => {
    if (!anime.value) return
    const trimmed = newCommentBody.value.trim()
    if (!trimmed) {
      postError.value = t('anime.ugc.bodyEmpty')
      return
    }
    if (runeLen(trimmed) > 2000) {
      postError.value = t('anime.ugc.bodyTooLong')
      return
    }
    posting.value = true
    postError.value = ''
    try {
      const resp = await commentApi.createComment(anime.value.id, trimmed)
      // REVIEW.md WR-03: prepend the newly-created comment to the list
      // instead of refetching page 1. Refetching destroys commentsNextCursor /
      // commentsHasMore and snaps users who had paginated through several
      // pages of comments back to the top. Prepending preserves cursor
      // state and lets infinite scroll continue working below the newly
      // posted card.
      const created: Comment | undefined = resp.data?.data || resp.data
      if (created?.id) {
        comments.value.unshift(created)
        commentsFetched.value = true
      }
      newCommentBody.value = ''
    } catch (err) {
      const e = err as ApiError
      const status = e.response?.status
      if (status === 429) {
        postError.value = t('anime.ugc.rateLimitError')
      } else if (status === 400) {
        const body = (e.response?.data?.error || e.response?.data?.message || '').toString().toLowerCase()
        if (body.includes('long') || body.includes('2000')) {
          postError.value = t('anime.ugc.bodyTooLong')
        } else {
          postError.value = t('anime.ugc.bodyEmpty')
        }
      } else {
        postError.value = t('anime.ugc.loadFailed')
      }
      console.error('Failed to post comment:', err)
    } finally {
      posting.value = false
    }
  }

  const startEditComment = (c: Comment) => {
    editingCommentId.value = c.id
    editingBody.value = c.body
    editError.value = ''
  }

  const cancelEditComment = () => {
    editingCommentId.value = null
    editingBody.value = ''
    editError.value = ''
  }

  const saveEditComment = async () => {
    if (!anime.value || !editingCommentId.value) return
    const trimmed = editingBody.value.trim()
    if (!trimmed) {
      editError.value = t('anime.ugc.bodyEmpty')
      return
    }
    if (runeLen(trimmed) > 2000) {
      editError.value = t('anime.ugc.bodyTooLong')
      return
    }
    const id = editingCommentId.value
    editSaving.value = true
    editError.value = ''
    try {
      const resp = await commentApi.updateComment(anime.value.id, id, trimmed)
      const updated: Comment = resp.data?.data || resp.data
      const idx = comments.value.findIndex((c) => c.id === id)
      if (idx !== -1 && updated && updated.id) {
        comments.value.splice(idx, 1, updated)
      } else if (idx !== -1) {
        // Server returned no body — patch the local copy with the new text.
        comments.value[idx] = { ...comments.value[idx], body: trimmed, updated_at: new Date().toISOString() }
      }
      cancelEditComment()
    } catch (err) {
      console.error('Failed to update comment:', err)
      editError.value = t('anime.ugc.editFailed')
    } finally {
      editSaving.value = false
    }
  }

  const deleteCommentItem = async (c: Comment) => {
    if (!anime.value) return
    if (!(await confirm({
      title: t('common.confirmTitle'),
      description: t('anime.ugc.deleteCommentConfirm'),
      confirmText: t('common.delete'),
      cancelText: t('common.cancel'),
      variant: 'destructive',
    }))) return
    const originalIdx = comments.value.findIndex((x) => x.id === c.id)
    if (originalIdx === -1) return
    const snapshot = comments.value[originalIdx]
    // Optimistic remove.
    comments.value.splice(originalIdx, 1)
    deleteError.value = ''
    try {
      await commentApi.deleteComment(anime.value.id, c.id)
    } catch (err) {
      console.error('Failed to delete comment:', err)
      // REVIEW.md WR-04: restore via id-keyed re-insertion rather than
      // splicing at the captured originalIdx. Between the optimistic
      // removal and the error path, the user may have triggered
      // loadMoreComments() (appends new entries) or saveEditComment()
      // (mutates in place), which would make originalIdx stale and put
      // the restored card at the wrong position. Find a stable anchor by
      // walking the current array and inserting before the first comment
      // strictly older than the snapshot, falling back to the front if
      // it's the newest or the array is empty. Newest-first ordering is
      // preserved without depending on a captured index.
      const insertAt = comments.value.findIndex((x) => {
        const a = new Date(snapshot.created_at).getTime()
        const b = new Date(x.created_at).getTime()
        return a > b || (a === b && snapshot.id > x.id)
      })
      if (insertAt === -1) {
        comments.value.push(snapshot)
      } else {
        comments.value.splice(insertAt, 0, snapshot)
      }
      deleteError.value = t('anime.ugc.deleteFailed')
      // Auto-clear after 5 seconds (mirrors the SPEC's "auto-dismiss 5s").
      setTimeout(() => {
        if (deleteError.value === t('anime.ugc.deleteFailed')) deleteError.value = ''
      }, 5000)
    }
  }

  // Watchers — Vue+Router URL-persistence pattern (RESEARCH.md Pattern 6).
  // Two-way: route.query.ugc → ugcTab handles deep links + back/forward;
  // ugcTab → router.replace + lazy fetch on first activation.
  watch(
    () => route.query.ugc,
    (v) => {
      const val = (typeof v === 'string' ? v : 'reviews') as UgcTab
      const normalized: UgcTab = (UGC_ALLOWED as readonly string[]).includes(val) ? val : 'reviews'
      if (normalized !== ugcTab.value) ugcTab.value = normalized
    }
  )

  watch(ugcTab, (v) => {
    if (route.query.ugc !== v) {
      router.replace({ query: { ...route.query, ugc: v } })
    }
    if (v === 'comments' && !commentsFetched.value && !commentsLoading.value) {
      void fetchComments()
    }
  })

  // Per-anime reset (route param change) — reset cache for new anime so a
  // stale list doesn't leak across navigations. Per-anime fetch is gated on
  // tab activation (or the deep-link ugc=comments path in loadAnimeData).
  function reset() {
    comments.value = []
    commentsHasMore.value = false
    commentsNextCursor.value = ''
    commentsError.value = ''
    commentsFetched.value = false
    newCommentBody.value = ''
    postError.value = ''
    editingCommentId.value = null
    editingBody.value = ''
    editError.value = ''
    deleteError.value = ''
  }

  return {
    ugcTab,
    comments,
    commentsHasMore,
    commentsLoading,
    commentsLoadingMore,
    commentsError,
    commentsFetched,
    newCommentBody,
    posting,
    postError,
    editingCommentId,
    editingBody,
    editError,
    editSaving,
    deleteError,
    fetchComments,
    loadMoreComments,
    postComment,
    startEditComment,
    cancelEditComment,
    saveEditComment,
    deleteCommentItem,
    reset,
  }
}
