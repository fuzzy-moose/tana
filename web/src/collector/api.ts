import { request } from '../api'

export interface FavoriteCategory {
  category: number
  name: string
  favorites: number
  entries_saved: number
  pages_saved: number
  last_saved_at?: string
  last_synced_at?: string
  state: 'idle' | 'queued' | 'running' | 'waiting_cooldown'
  full: boolean
  queued: boolean
  queued_full: boolean
  started_at?: string
  finished_at?: string
  last_outcome?: 'success' | 'failed'
  last_error?: string
  last_error_at?: string
  retry_at?: string
}

export interface CollectorStatus {
  favorites: { host: string, account_key: string, categories: FavoriteCategory[], downloads?: FavoriteDownloadsStatus }
  inventory: {
    gallery_references: number
    metadata_available: number
    metadata_pending: number
    metadata_failed: number
    fetches_pending: number
    fetches_failed: number
  }
  metadata_errors: { gallery_id: number, error: string, at?: string }[]
  metadata_last_error?: string
  metadata_retry_at?: string
  upstream_cooldown_until?: string
}

export interface FavoriteDownloadsStatus {
  categories: number[]
  baseline_state: 'not_started' | 'collecting' | 'ready'
  baseline_categories: number
}

export interface ConnectionStatus {
  configured: boolean
  reachable: boolean
  authenticated: boolean | null
  checked_at: string
  error?: string
  status?: CollectorStatus
}

export const collectorMessages: Record<string, string> = {
  collector_not_configured: 'No collector is configured for Tana.',
  collector_unreachable: 'Could not reach the collector. Check that it is running and connected.',
  collector_unauthorized: 'The collector rejected Tana’s credentials. Check the configured API token.',
  collector_status_unavailable: 'The collector is reachable, but its status API is unavailable.',
  collector_invalid_response: 'The collector returned an unexpected status response.',
  collector_unavailable: 'The request could not be confirmed. Check collector status and try again.',
  invalid_download_categories: 'Select distinct favorite categories from 0 to 9.',
}

export const getCollectorStatus = (signal: AbortSignal) => request<ConnectionStatus>('/api/collector/status', { signal })

export const syncFavorites = (category: string, full: boolean, signal: AbortSignal) => request('/api/collector/favorites/' + category + '/sync', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ full }),
  signal,
}, collectorMessages)

export const saveFavoriteDownloadSettings = (categories: number[], signal: AbortSignal) => request<{ categories: number[] }>('/api/collector/favorites/download-settings', {
  method: 'PUT',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ categories }),
  signal,
}, collectorMessages)
