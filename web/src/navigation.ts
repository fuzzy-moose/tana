import { useSyncExternalStore } from 'react'

function subscribe(callback: () => void) {
  window.addEventListener('hashchange', callback)
  return () => window.removeEventListener('hashchange', callback)
}

export type Route =
  | { kind: 'galleries', search: string, page: number }
  | { kind: 'libraries' | 'not-found' }
  | { kind: 'detail' | 'reader', id: string, page: number }

export function useRoute(): Route {
  const hash = useSyncExternalStore(subscribe, () => window.location.hash)
  const [path, query] = hash.slice(1).split('?')
  const params = new URLSearchParams(query)
  const value = Number(params.get('page') ?? 1)
  const page = Number.isSafeInteger(value) && value > 0 ? value : 1
  if (!path || path === '/' || path === '/galleries') return { kind: 'galleries', search: params.get('q') ?? '', page }
  if (path === '/libraries' || path === 'library') return { kind: 'libraries' }
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
export const listingLink = (search: string, page = 1) => `#/galleries?${new URLSearchParams({ q: search, page: String(page) })}`
