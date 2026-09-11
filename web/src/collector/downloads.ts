import { request } from '../api'
import { collectorMessages } from './api'

export interface DownloadJob {
  gallery_id: number
  state: 'queued' | 'running' | 'completed' | 'failed' | 'cancelled' | 'deleting'
  created_at: string
  updated_at: string
  retry_at?: string
  failures: number
  size_bytes: number
  error?: string
}

export const downloadMessages: Record<string, string> = {
  ...collectorMessages,
  collector_unavailable: 'The download request could not be confirmed. Check collector status and try again.',
  invalid_gallery_reference: 'Enter a Panda gallery URL containing its gallery ID and token.',
  download_not_found: 'This download is no longer available. Refresh the download list.',
  download_state_conflict: 'The download state changed. Check the refreshed list and try again.',
  download_token_conflict: 'This gallery already has a download with a different token. Delete that job before submitting the corrected URL.',
}

export function parseGalleryURL(value: string): { gid: number, token: string } {
  try {
    const url = new URL(value.trim())
    const match = /^\/g\/(\d+)\/([^/]+)\/?$/.exec(url.pathname)
    if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || !match) throw new Error()
    const gid = Number(match[1])
    const token = decodeURIComponent(match[2])
    if (!Number.isSafeInteger(gid) || gid <= 0 || !token || token.length > 256 || /[\s/]/.test(token)) throw new Error()
    return { gid, token }
  } catch {
    throw new Error('Enter a Panda gallery URL in the form /g/12345/token/.')
  }
}

const base = '/api/collector/downloads'
export const downloadPageSize = 25
export const listDownloads = (offset: number, signal: AbortSignal) => request<{ jobs: DownloadJob[] }>(
  `${base}?limit=${downloadPageSize + 1}&offset=${offset}`, { signal }, downloadMessages,
)
export const submitDownload = (ref: { gid: number, token: string }, signal: AbortSignal) => request<DownloadJob>(base, {
  method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(ref), signal,
}, downloadMessages)
export const changeDownload = (id: number, action: 'cancel' | 'retry' | 'delete', signal: AbortSignal) => request<DownloadJob | undefined>(
  `${base}/${id}${action === 'delete' ? '' : '/' + action}`, { method: action === 'delete' ? 'DELETE' : 'POST', signal }, downloadMessages,
)
export const downloadFileURL = (id: number) => `${base}/${id}/file`

export function downloadState(job: DownloadJob) {
  if (job.state === 'queued' && job.retry_at) return job.error === 'panda_banned' ? 'Waiting for Panda cooldown' : 'Waiting to retry'
  return { queued: 'Queued', running: 'Running', completed: 'Available on collector', failed: 'Failed', cancelled: 'Cancelled', deleting: 'Deleting' }[job.state] ?? job.state
}

export function downloadFailure(code: string) {
  const messages: Record<string, string> = {
    archive_unavailable_or_unauthenticated: 'Archive unavailable or Panda authentication required.',
    archive_url_expired: 'Archive link expired.',
    invalid_zip: 'The downloaded file was not a valid ZIP archive.',
    panda_banned: 'Panda requests are paused for a cooldown.',
    storage_error: 'The collector could not write the archive. Check its storage.',
    transfer_failed: 'Archive transfer failed.',
  }
  if (code.startsWith('upstream_http_')) return `Panda returned HTTP ${code.slice('upstream_http_'.length)}.`
  return messages[code] ?? code
}
