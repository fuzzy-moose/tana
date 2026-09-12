import { request } from '../api'
import { collectorMessages } from '../collector/api'
import type { SearchCompletion } from '../galleries/api'

export interface PandaGallery {
  gallery_id: number
  title: string
  thumbnail_url: string
  page_count: number
  posted_at: string
  url: string
}

export interface PandaCatalogResult {
  items: PandaGallery[]
  total: number
  page: number
  page_size: number
  total_pages: number
}

export interface FeedStatus {
  capture_active: boolean
  last_captured_at?: string
  last_capture_error?: string
  last_capture_error_at?: string
  processing_pending: number
  processing_error?: string
  continuity: 'unknown' | 'overlap' | 'possible_gap'
  possible_gaps: number
}

export function listPandaCatalog(search: string, page: number, pageSize: number, includeExpunged: boolean, signal?: AbortSignal) {
  const params = new URLSearchParams({ q: search, page: String(page), page_size: String(pageSize), include_expunged: String(includeExpunged) })
  return request<PandaCatalogResult>(`/api/collector/catalog?${params}`, { signal }, {
    ...collectorMessages,
    invalid_query: 'Invalid search query.',
  })
}

export function completePandaSearch(query: string, cursor: number, signal?: AbortSignal) {
  return request<SearchCompletion>(`/api/collector/catalog/completions?${new URLSearchParams({ q: query, cursor: String(cursor) })}`, { signal }, collectorMessages)
}

export const getFeedStatus = (signal: AbortSignal) => request<FeedStatus>('/api/collector/feed/status', { signal }, collectorMessages)

export const refreshFeed = (signal: AbortSignal) => request<FeedStatus>('/api/collector/feed/refresh', {
  method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}', signal,
}, collectorMessages)
