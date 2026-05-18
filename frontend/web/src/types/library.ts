/**
 * Phase 5 (LIB-09) — Raw Library admin UI types.
 *
 * Mirrors the Go domain structs in `services/library/internal/domain/*`
 * and the SPEC-locked /api/library/* HTTP body shapes. Snake_case keys
 * match the JSON the backend emits via httputil.OK; consumers in
 * RawLibrary.vue destructure response.data.
 */

export type JobStatus =
  | 'queued'
  | 'downloading'
  | 'encoding'
  | 'uploading'
  | 'done'
  | 'failed'
  | 'cancelled'

export type JobSource = 'nyaa' | 'animetosho' | 'manual'

export interface Job {
  id: string
  source: JobSource
  magnet: string
  title: string
  uploader?: string
  quality?: string
  size_bytes: number
  shikimori_id?: string
  status: JobStatus
  progress_pct: number
  error_text?: string
  created_at: string
  updated_at: string
  completed_at?: string
}

/**
 * Release is the unified search-result row from the library aggregator
 * (Nyaa + AnimeTosho). The `source` field tags origin so the UI can
 * render the provider chip in the right colour.
 */
export interface Release {
  title: string
  magnet: string
  uploader?: string
  quality?: string
  size_bytes: number
  source: 'nyaa' | 'animetosho'
  mal_id?: number
  found_at: string
}

export interface Episode {
  id: string
  shikimori_id: string
  episode_number: number
  job_id?: string
  minio_path: string
  duration_sec?: number
  size_bytes?: number
  created_at: string
}

export interface LibraryHealth {
  disk_free_bytes: number
  disk_total_bytes: number
  active_torrents: number
  active_jobs_by_status: Record<string, number>
}

export interface CreateJobPayload {
  magnet: string
  title: string
  source: JobSource
  uploader?: string
  quality?: string
  size_bytes?: number
  shikimori_id?: string
}
