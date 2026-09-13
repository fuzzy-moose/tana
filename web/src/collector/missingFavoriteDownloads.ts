import { request } from '../api'
import { downloadMessages } from './downloads'

export interface MissingFavoriteDownloadCounts {
  category: number
  total: number
  present: number
  missing: number
  new_downloads: number
  existing_jobs: number
  retained_archives: number
  failed: number
  cancelled: number
  deleting: number
}

export interface MissingFavoriteDownloadPreview extends MissingFavoriteDownloadCounts {
  plan_id: string
}

const messages = {
  ...downloadMessages,
  missing_download_preview_invalid: 'This preview is no longer valid. Refresh the preview before confirming.',
}

const path = (category: number) => `/api/collector/favorites/${category}/missing-downloads`

export const previewMissingFavoriteDownloads = (category: number, signal: AbortSignal) => request<MissingFavoriteDownloadPreview>(
  `${path(category)}/preview`, { method: 'POST', signal }, messages,
)

export const submitMissingFavoriteDownloads = (category: number, planID: string, signal: AbortSignal) => request<MissingFavoriteDownloadCounts>(path(category), {
  method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ plan_id: planID }), signal,
}, messages)
