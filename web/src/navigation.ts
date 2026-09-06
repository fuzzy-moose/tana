import { useSyncExternalStore } from 'react'

function subscribe(callback: () => void) {
  window.addEventListener('hashchange', callback)
  return () => window.removeEventListener('hashchange', callback)
}

export type Route =
  | { kind: 'galleries', search: string, page: number }
  | { kind: 'libraries' | 'collector' | 'not-found' }
  | { kind: 'detail', id: number, page: number }
  | { kind: 'reader', id: number, page: number, lastPage?: number }

export function useRoute(): Route {
  const hash = useSyncExternalStore(subscribe, () => window.location.hash)
  const [path, query] = hash.slice(1).split('?')
  const params = new URLSearchParams(query)
  const value = Number(params.get('page') ?? 1)
  const page = Number.isSafeInteger(value) && value > 0 ? value : 1
  if (!path || path === '/' || path === '/galleries') return { kind: 'galleries', search: params.get('q') ?? '', page }
  if (path === '/libraries' || path === 'library') return { kind: 'libraries' }
  if (path === '/collector') return { kind: 'collector' }
  const spread = path.match(/^\/galleries\/(\d+)\/page\/(\d+)(?:-(\d+))?$/)
  if (spread) {
    const id = Number(spread[1])
    const first = Number(spread[2])
    const last = Number(spread[3] ?? spread[2])
    if (!Number.isSafeInteger(id) || id < 1) return { kind: 'not-found' }
    if (!Number.isSafeInteger(first) || first < 1 || !Number.isSafeInteger(last) || (last !== first && last !== first + 1)) return { kind: 'not-found' }
    return { kind: 'reader', id, page: first, lastPage: last }
  }
  const match = path.match(/^\/galleries\/(\d+)(\/read)?$/)
  if (match) {
    const id = Number(match[1])
    if (!Number.isSafeInteger(id) || id < 1) return { kind: 'not-found' }
    return { kind: match[2] ? 'reader' : 'detail', id, page }
  }
  return { kind: 'not-found' }
}

export const galleryLink = (id: number) => `#/galleries/${id}`
export const readerLink = (id: number, page = 1) => `${galleryLink(id)}/read?page=${page}`
// Entry links let image dimensions determine the spread; saved URLs identify its exact pages.
export const readerSpreadLink = (id: number, first: number, last = first) => `${galleryLink(id)}/page/${first === last ? first : `${first}-${last}`}`
export function listingLink(search: string, page = 1) {
  const params = new URLSearchParams()
  if (search) params.set('q', search)
  if (page > 1) params.set('page', String(page))
  return `#/galleries${params.size ? `?${params}` : ''}`
}
