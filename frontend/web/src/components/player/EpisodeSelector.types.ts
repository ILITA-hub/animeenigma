/** One episode entry rendered by EpisodeSelector.vue. Each player normalizes
 *  its native episode model into this shape. */
export interface EpisodeOption {
  /** Unique key — v-for key, selection match, and Phase-C data-wt-id. */
  key: string | number
  /** Text shown on the button (episode number / ordinal). */
  label: string | number
  /** Ordinal for watched comparison: watched when number <= watchedUpTo. */
  number: number
  /** Optional filler episode — dimmed. */
  isFiller?: boolean
  /** Optional provider-supplied episode title (tooltips / player header). */
  title?: string
}
