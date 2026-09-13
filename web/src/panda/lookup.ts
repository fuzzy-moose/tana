import { request } from '../api'
import { collectorMessages } from '../collector/api'
import { parseGalleryURL } from '../collector/downloads'

export interface PandaMetadata {
  gid: number
  token: string
  error: string
  title: string
  title_jpn: string
  category: string
  thumb: string
  uploader: string
  posted: string
  filecount: string
  filesize: number
  expunged: boolean
  rating: string
  torrentcount: string
  torrents: { hash: string, added: string, name: string, tsize: string, fsize: string }[] | null
  tags: string[] | null
  parent_gid: string
  parent_key: string
  current_gid: string
  current_key: string
  first_gid: string
  first_key: string
}

export interface MetadataFetchJob {
  id: string
  status: 'pending' | 'completed'
  created_at: string
  completed_at?: string
  entries: { gid: number, status: 'pending' | 'successful' | 'failed', error?: string, refreshed_at?: string }[]
}

export interface PandaLookupResult {
  gid: number
  token: string
  url: string
  metadata?: PandaMetadata
  refreshed_at?: string
  unverified?: boolean
  fetch_job?: MetadataFetchJob
}

const messages = {
  ...collectorMessages,
  collector_unavailable: 'Collector unavailable. Try again when it is reachable.',
  gallery_not_found: 'Gallery not found in the collector.',
  invalid_gallery_reference: 'Enter a positive gallery ID or a Panda gallery URL.',
  metadata_fetch_not_found: 'This metadata request is no longer available. Look up the gallery again.',
}

export function parseLookup(value: string): { gid: number, token?: string } {
  const trimmed = value.trim()
  if (/^\d+$/.test(trimmed)) {
    const gid = Number(trimmed)
    if (Number.isSafeInteger(gid) && gid > 0) return { gid }
  } else {
    try { return parseGalleryURL(trimmed) } catch { /* Report the combined input format. */ }
  }
  throw new Error(messages.invalid_gallery_reference)
}

export const lookupPanda = (ref: { gid: number, token?: string }, signal: AbortSignal) => request<PandaLookupResult>('/api/collector/catalog/lookup', {
  method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(ref), signal,
}, messages)

export const getMetadataFetch = (id: string, signal: AbortSignal) => request<MetadataFetchJob>(`/api/collector/metadata/fetches/${encodeURIComponent(id)}`, { signal }, messages)
