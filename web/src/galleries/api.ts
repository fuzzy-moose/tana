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

export function listGalleries(search: string, page: number, pageSize: number, signal?: AbortSignal) {
  return request<GalleryListing>(`/api/galleries?${new URLSearchParams({ q: search, page: String(page), page_size: String(pageSize) })}`, { signal })
}

export function getGallery(id: string, signal?: AbortSignal) {
  return request<GalleryDetails>(`/api/galleries/${encodeURIComponent(id)}`, { signal }, {
    not_found: 'This gallery no longer exists. Return to Galleries to choose another.',
  })
}

export const imageURL = (id: string, page: number) => `/api/galleries/${encodeURIComponent(id)}/pages/${page}/image`
