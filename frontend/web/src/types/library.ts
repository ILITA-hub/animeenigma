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

export type JobSource = 'nyaa' | 'animetosho' | 'manual' | 'jackett'

/** Storage backend a job targets / an episode row actually lives on.
 *  `''` (empty) on a job means "use the default (minio)". */
export type StorageBackend = 'minio' | 's3'

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
  /** Requested storage-backend override at job creation; '' = default (minio). */
  storage?: '' | StorageBackend
}

/**
 * Release is the unified search-result row from the library search.
 * `source` tags origin so the UI can render the provider chip in the right
 * colour: `jackett` is the multi-indexer primary tier (carries `seeders`),
 * `nyaa`/`animetosho` are the fallback tier.
 */
export interface Release {
  title: string
  magnet: string
  uploader?: string
  quality?: string
  size_bytes: number
  source: 'nyaa' | 'animetosho' | 'jackett'
  mal_id?: number
  /** Live swarm seeders — only Jackett populates this (omitted otherwise). */
  seeders?: number
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
  /** Backend this row's minio_path prefix actually lives on. */
  storage: StorageBackend
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
  /** Destination storage backend; '' (or omitted) = default (minio). */
  storage?: '' | StorageBackend
}

/**
 * File manager (Task 7+) — admin browse/delete/download over the torrent
 * working dir (domain=work) and the two object-store backends (domain=minio|s3).
 * Mirrors `browseResponseDTO`/`fileEntryDTO`/`fileEpisodeDTO` in
 * `services/library/internal/handler/files.go`.
 */
export type FileDomain = 'work' | 'minio' | 's3'

/** Annotates a synthesized object-store folder with the library_episodes row
 *  that owns it. */
export interface FileEpisode {
  episode_id: string
  shikimori_id: string
  episode?: number
  source: string
  freshness: 'fresh' | 'stale'
}

/** One row in a Browse listing: a directory or a file/object. */
export interface FileEntry {
  name: string
  kind: 'dir' | 'file'
  size: number
  key?: string
  episode?: FileEpisode
}

/** Browse response body. */
export interface BrowseResponse {
  domain: FileDomain
  prefix: string
  breadcrumb: string[]
  entries: FileEntry[]
}
