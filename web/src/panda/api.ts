import { request } from '../api'
import { collectorMessages } from '../collector/api'
import type { SearchCompletion, TagSuggestion } from '../galleries/api'
import type { PandaListing } from '../navigation'
import type { ScoreSource } from './score'

/** A tag offered to a card, with its weight in the active Panda score model. */
export interface PandaSummaryTag extends TagSuggestion {
  weight?: number
  source?: ScoreSource
  /** The tag names a favorite author. */
  favorite_author?: boolean
}

export interface PandaGallery {
  gallery_id: number
  title: string
  thumbnail_url: string
  page_count: number | null
  posted_at: string | null
  /** The Panda gallery category in lowercase; absent while unknown. */
  category?: string
  metadata_source: 'feed' | 'collected'
  metadata_state?: 'waiting' | 'failed'
  fetch_eligible_at?: string
  first_seen_at?: string
  url: string
  summary_tags: PandaSummaryTag[]
  displayed?: boolean
  score?: PandaGalleryScore
}

export interface PandaScoreWeight {
  kind: 'tag' | 'uploader' | 'category'
  label: string
  weight: number
  source?: ScoreSource
}

/** The score among the top quarter, or else the top half, of the held-out favorite scores. */
export type ScoreGrade = 'top_quarter' | 'top_half'

/** What the score filter knows about a gallery: its score, its strongest weights, its score grade and whether it is below the threshold. */
export interface PandaGalleryScore {
  value?: number
  weights?: PandaScoreWeight[]
  /** The held-out favorite scores above this score; absent without any. */
  favorites_above?: number
  grade?: ScoreGrade
  rule?: 'below_threshold'
}

/** Present while the score filter is in force. */
export interface PandaCatalogScoreFilter {
  threshold_tenths?: number
  hidden_unseen?: boolean
  /** The number of held-out favorite scores that the score grades compare with. */
  held_out_favorites?: number
}

export interface PandaCatalogResult {
  items: PandaGallery[]
  page_size: number
  next_cursor?: string
  previous_cursor?: string
  score_filter?: PandaCatalogScoreFilter
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

export function listPandaCatalog({ search, cursor, includeExpunged, categories, bypassDefault, view, scoreFilter }: PandaListing, pageSize: number, signal?: AbortSignal) {
  const params = new URLSearchParams({ q: search, cursor, page_size: String(pageSize), include_expunged: String(includeExpunged) })
  for (const category of categories) params.append('category', category)
  if (bypassDefault) params.set('bypass_default', 'true')
  if (scoreFilter !== 'apply') params.set('score_filter', scoreFilter)
  if (view === 'unseen') params.set('unseen', 'true')
  return request<PandaCatalogResult>(`/api/collector/catalog?${params}`, { signal }, {
    ...collectorMessages,
    invalid_query: 'Invalid search query.',
    invalid_category: 'Unknown Panda gallery category.',
    invalid_pagination: 'Invalid catalog position. Return to the newest galleries.',
  })
}

// No abort signal: leaving the page must not cancel a record.
export function recordPandaDisplays(galleryIDs: number[]) {
  return request<void>('/api/collector/catalog/displays', {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ gallery_ids: galleryIDs }), keepalive: true,
  }, collectorMessages)
}

export function catchUpPanda(galleryID: number, signal: AbortSignal) {
  return request<void>('/api/collector/catalog/catch-up', {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ gallery_id: galleryID }), signal,
  }, { ...collectorMessages, gallery_not_found: 'This gallery is no longer in the Panda catalog. Reload and try again.' })
}

export function completePandaSearch(query: string, cursor: number, signal?: AbortSignal) {
  return request<SearchCompletion>(`/api/collector/catalog/completions?${new URLSearchParams({ q: query, cursor: String(cursor) })}`, { signal }, collectorMessages)
}

export const getFeedStatus = (signal: AbortSignal) => request<FeedStatus>('/api/collector/feed/status', { signal }, collectorMessages)

export const refreshFeed = (signal: AbortSignal) => request<FeedStatus>('/api/collector/feed/refresh', {
  method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}', signal,
}, collectorMessages)
