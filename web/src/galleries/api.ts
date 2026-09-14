import { request } from '../api'
import type { GallerySort } from '../navigation'

export interface Gallery {
  id: number
  title: string
  page_count: number
}

export interface GalleryDetails extends Gallery {
  tags: TagSuggestion[]
  panda_candidate_id?: number
}

export interface GalleryListing {
  items: Gallery[]
  total: number
  page: number
  page_size: number
}

export interface TagSuggestion {
  namespace: string
  value: string
  term: string
}

export interface SearchCompletion {
  start: number
  end: number
  items: TagSuggestion[]
}

export function completeGallerySearch(query: string, cursor: number, signal?: AbortSignal) {
  return request<SearchCompletion>(`/api/gallery-search/completions?${new URLSearchParams({ q: query, cursor: String(cursor) })}`, { signal })
}

export function listGalleries(search: string, page: number, pageSize: number, categories: string[], sort: GallerySort, signal?: AbortSignal) {
  const params = new URLSearchParams({ q: search, page: String(page), page_size: String(pageSize) })
  for (const category of categories) params.append('category', category)
  if (sort !== 'title') params.set('sort', sort)
  return request<GalleryListing>(`/api/galleries?${params}`, { signal }, {
    invalid_query: 'Invalid search query.',
    invalid_category: 'Unknown Panda gallery category.',
  })
}

export function getGallery(id: number, signal?: AbortSignal) {
  return request<GalleryDetails>(`/api/galleries/${id}`, { signal }, {
    not_found: 'This gallery no longer exists. Return to Galleries to choose another.',
  })
}

export const imageURL = (id: number, page: number) => `/api/galleries/${id}/pages/${page}/image`
