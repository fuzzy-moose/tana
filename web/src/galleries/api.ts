import { request } from '../api'

export interface Gallery {
  id: string
  title: string
  page_count: number
}

export interface GalleryDetails extends Gallery {
  tags: { namespace: string, value: string }[]
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

export function listGalleries(search: string, page: number, pageSize: number, signal?: AbortSignal) {
  return request<GalleryListing>(`/api/galleries?${new URLSearchParams({ q: search, page: String(page), page_size: String(pageSize) })}`, { signal }, {
    invalid_query: 'Invalid search query.',
  })
}

export function getGallery(id: string, signal?: AbortSignal) {
  return request<GalleryDetails>(`/api/galleries/${encodeURIComponent(id)}`, { signal }, {
    not_found: 'This gallery no longer exists. Return to Galleries to choose another.',
  })
}

export const imageURL = (id: string, page: number) => `/api/galleries/${encodeURIComponent(id)}/pages/${page}/image`
