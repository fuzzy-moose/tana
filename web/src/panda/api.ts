import { request } from '../api'
import { collectorMessages } from '../collector/api'
import type { SearchCompletion, TagSuggestion } from '../galleries/api'
import { isVerdictView } from '../navigation'
import type { PandaListing, PandaScoreFilter } from '../navigation'
import type { ScoreRule } from './score'

export type PandaVerdict = 'interesting' | 'not_for_me'

export const verdictLabels: Record<PandaVerdict, string> = { not_for_me: 'Not for me', interesting: 'Interesting' }

/** A tag offered to a card, with its weight in the active Panda score model. */
export interface PandaSummaryTag extends TagSuggestion {
  weight?: number
  /** The rule of the tag's score override. */
  override?: ScoreRule
}

export interface PandaGallery {
  gallery_id: number
  title: string
  thumbnail_url: string
  page_count: number | null
  posted_at: string | null
  metadata_source: 'feed' | 'collected'
  metadata_state?: 'waiting' | 'failed'
  fetch_eligible_at?: string
  first_seen_at?: string
  url: string
  summary_tags: PandaSummaryTag[]
  displayed?: boolean
  verdict?: PandaVerdict
  score?: PandaGalleryScore
}

export interface PandaScoreWeight {
  kind: 'tag' | 'uploader' | 'category'
  label: string
  weight: number
}

/** What the score filter knows about a gallery; the verdict rule is left to the card's own verdict. */
export interface PandaGalleryScore {
  value?: number
  weights?: PandaScoreWeight[]
  rule?: 'never_hide' | 'always_hide' | 'below_threshold'
  override?: string
}

/** Present while the score filter is in force. */
export interface PandaCatalogScoreFilter {
  threshold_tenths?: number
  hidden_unseen?: boolean
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
  if (scoreFilter !== 'apply' && !isVerdictView(view)) params.set('score_filter', scoreFilter)
  if (view === 'unseen') params.set('unseen', 'true')
  else if (isVerdictView(view)) params.set('verdict', view)
  return request<PandaCatalogResult>(`/api/collector/catalog?${params}`, { signal }, {
    ...collectorMessages,
    invalid_query: 'Invalid search query.',
    invalid_category: 'Unknown Panda gallery category.',
    invalid_pagination: 'Invalid catalog position. Return to the newest galleries.',
  })
}

// No abort signal: leaving the page must not cancel a record.
// Displays in the "hidden only" state still mark galleries seen, but are not score evidence.
export function recordPandaDisplays(galleryIDs: number[], scoreFilter: PandaScoreFilter = 'apply') {
  const body = scoreFilter === 'hidden' ? { gallery_ids: galleryIDs, score_filter: 'hidden' } : { gallery_ids: galleryIDs }
  return request<void>('/api/collector/catalog/displays', {
    method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body), keepalive: true,
  }, collectorMessages)
}

// No abort signal: leaving the page must not cancel a save. `null` removes the verdict.
export function savePandaVerdict(galleryID: number, verdict: PandaVerdict | null) {
  const path = `/api/collector/catalog/verdicts/${galleryID}`
  return request<void>(path, verdict === null
    ? { method: 'DELETE', keepalive: true }
    : { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ verdict }), keepalive: true }, collectorMessages)
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
