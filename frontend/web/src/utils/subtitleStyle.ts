/**
 * Shared subtitle appearance mapping.
 *
 * The appearance-panel live preview (`SubtitlesMenu`) and the actual on-video
 * renderer (`SubtitleOverlay`) must agree on how the user's prefs map to CSS —
 * the preview exists precisely to show what will render. Keeping the transform
 * here (single source of truth) stops the two surfaces from silently drifting.
 */

/**
 * Map the user's "Background" opacity preference (0–100 %) to the subtitle
 * box's CSS background color. The 0.85 ceiling keeps even "100 %" from fully
 * occluding the video behind wide subtitle boxes.
 */
export function subtitleBgColor(bgOpacityPct: number): string {
  const alpha = ((bgOpacityPct / 100) * 0.85).toFixed(2)
  return `rgba(0, 0, 0, ${alpha})`
}
