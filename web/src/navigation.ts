import { useSyncExternalStore } from 'react'

function subscribe(callback: () => void) {
  window.addEventListener('hashchange', callback)
  return () => window.removeEventListener('hashchange', callback)
}

export type Route =
  | { kind: 'galleries', search: string, page: number }
  | { kind: 'libraries' | 'not-found' }
  | { kind: 'detail', id: string, page: number }
  | { kind: 'reader', id: string, page: number, lastPage?: number }

export function useRoute(): Route {
  const hash = useSyncExternalStore(subscribe, () => window.location.hash)
  const [path, query] = hash.slice(1).split('?')
  const params = new URLSearchParams(query)
  const value = Number(params.get('page') ?? 1)
  const page = Number.isSafeInteger(value) && value > 0 ? value : 1
  if (!path || path === '/' || path === '/galleries') return { kind: 'galleries', search: params.get('q') ?? '', page }
  if (path === '/libraries' || path === 'library') return { kind: 'libraries' }
  const spread = path.match(/^\/galleries\/([^/]+)\/page\/(\d+)(?:-(\d+))?$/)
  if (spread) {
    const first = Number(spread[2])
    const last = Number(spread[3] ?? spread[2])
    if (!Number.isSafeInteger(first) || first < 1 || !Number.isSafeInteger(last) || (last !== first && last !== first + 1)) return { kind: 'not-found' }
    try {
      return { kind: 'reader', id: decodeURIComponent(spread[1]), page: first, lastPage: last }
    } catch { return { kind: 'not-found' } }
  }
  const match = path.match(/^\/galleries\/([^/]+)(\/read)?$/)
  if (match) {
    try {
      return { kind: match[2] ? 'reader' : 'detail', id: decodeURIComponent(match[1]), page }
    } catch { /* A malformed link has no matching gallery. */ }
  }
  return { kind: 'not-found' }
}

export const galleryLink = (id: string) => `#/galleries/${encodeURIComponent(id)}`
export const readerLink = (id: string, page = 1) => `${galleryLink(id)}/read?page=${page}`
// Entry links let image dimensions determine the spread; saved URLs identify its exact pages.
export const readerSpreadLink = (id: string, first: number, last = first) => `${galleryLink(id)}/page/${first === last ? first : `${first}-${last}`}`
export function listingLink(search: string, page = 1) {
  const params = new URLSearchParams()
  if (search) params.set('q', search)
  if (page > 1) params.set('page', String(page))
  return `#/galleries${params.size ? `?${params}` : ''}`
}
