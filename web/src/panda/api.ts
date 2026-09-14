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
  page_size: number
  next_cursor?: string
  previous_cursor?: string
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

export interface PandaDefaultFilter {
  query: string
  categories: string[]
}

export function hasDefaultFilter(filter: PandaDefaultFilter | null) {
  return !!filter && (!!filter.query || filter.categories.length > 0)
}

export function getPandaDefaultFilter(signal?: AbortSignal) {
  return request<PandaDefaultFilter>('/api/panda/default-filter', { signal })
}

export function savePandaDefaultFilter(filter: PandaDefaultFilter) {
  return request<PandaDefaultFilter>('/api/panda/default-filter', {
    method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(filter),
  }, { ...collectorMessages, invalid_query: 'Invalid search query.', invalid_category: 'Unknown Panda gallery category.' })
}

export function listPandaCatalog(search: string, cursor: string, pageSize: number, includeExpunged: boolean, categories: string[], bypassDefault: boolean, signal?: AbortSignal) {
  const params = new URLSearchParams({ q: search, cursor, page_size: String(pageSize), include_expunged: String(includeExpunged) })
  for (const category of categories) params.append('category', category)
  if (bypassDefault) params.set('bypass_default', 'true')
  return request<PandaCatalogResult>(`/api/collector/catalog?${params}`, { signal }, {
    ...collectorMessages,
    invalid_query: 'Invalid search query.',
    invalid_category: 'Unknown Panda gallery category.',
    invalid_pagination: 'Invalid catalog position. Return to the newest galleries.',
  })
}

export function completePandaSearch(query: string, cursor: number, signal?: AbortSignal) {
  return request<SearchCompletion>(`/api/collector/catalog/completions?${new URLSearchParams({ q: query, cursor: String(cursor) })}`, { signal }, collectorMessages)
}

export const getFeedStatus = (signal: AbortSignal) => request<FeedStatus>('/api/collector/feed/status', { signal }, collectorMessages)

export const refreshFeed = (signal: AbortSignal) => request<FeedStatus>('/api/collector/feed/refresh', {
  method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}', signal,
}, collectorMessages)
