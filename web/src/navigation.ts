import { useMemo, useSyncExternalStore } from 'react'

function subscribe(callback: () => void) {
  window.addEventListener('hashchange', callback)
  return () => window.removeEventListener('hashchange', callback)
}

export type CollectorPage = 'overview' | 'downloads' | 'favorites' | 'sitemap' | 'imports' | 'proxies' | 'feeds'
export type GallerySort = 'title' | 'favorited_desc' | 'favorited_asc'

export type Route =
  | { kind: 'galleries', search: string, page: number, categories: string[], sort: GallerySort }
  | { kind: 'panda', search: string, page: number, includeExpunged: boolean, categories: string[], bypassDefault: boolean }
  | { kind: 'libraries' | 'source-cleanup' | 'panda-lookup' | 'not-found' }
  | { kind: 'library-refresh', libraryID?: number }
  | { kind: 'collector', page: CollectorPage }
  | { kind: 'detail', id: number, page: number }
  | { kind: 'reader', id: number, page: number, lastPage?: number }

export function useRoute(): Route {
  const hash = useSyncExternalStore(subscribe, () => window.location.hash)
  const [path, query] = hash.slice(1).split('?')
  // Page-only navigation must retain the category identity used by resize anchors.
  const categoryKey = String(JSON.stringify(new URLSearchParams(query).getAll('category')))
  const categories = useMemo<string[]>(() => JSON.parse(categoryKey), [categoryKey])
  const params = new URLSearchParams(query)
  const value = Number(params.get('page') ?? 1)
  const page = Number.isSafeInteger(value) && value > 0 ? value : 1
  const sort = params.get('sort')
  if (!path || path === '/' || path === '/galleries') return { kind: 'galleries', search: params.get('q') ?? '', page, categories, sort: sort === 'favorited_desc' || sort === 'favorited_asc' ? sort : 'title' }
  if (path === '/panda') return { kind: 'panda', search: params.get('q') ?? '', page, includeExpunged: params.get('include_expunged') === 'true', categories, bypassDefault: params.get('bypass_default') === 'true' }
  if (path === '/panda/lookup') return { kind: 'panda-lookup' }
  if (path === '/libraries' || path === 'library') return { kind: 'libraries' }
  if (path === '/libraries/source-cleanup') return { kind: 'source-cleanup' }
  if (path === '/libraries/refresh') return { kind: 'library-refresh' }
  const refresh = path.match(/^\/libraries\/(\d+)\/refresh$/)
  if (refresh) {
    const libraryID = Number(refresh[1])
    return Number.isSafeInteger(libraryID) && libraryID > 0 ? { kind: 'library-refresh', libraryID } : { kind: 'not-found' }
  }
  if (path === '/collector') return { kind: 'collector', page: 'overview' }
  const collector = path.match(/^\/collector\/(downloads|favorites|sitemap|imports|proxies|feeds)$/)
  if (collector) return { kind: 'collector', page: collector[1] as CollectorPage }
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
export function listingLink(search: string, page = 1, categories: string[] = [], sort: GallerySort = 'title') {
  const params = new URLSearchParams()
  if (search) params.set('q', search)
  if (page > 1) params.set('page', String(page))
  for (const category of categories) params.append('category', category)
  if (sort !== 'title') params.set('sort', sort)
  return `#/galleries${params.size ? `?${params}` : ''}`
}

export function pandaListingLink(search: string, page = 1, includeExpunged = false, categories: string[] = [], bypassDefault = false) {
  const params = new URLSearchParams()
  if (search) params.set('q', search)
  if (page > 1) params.set('page', String(page))
  if (includeExpunged) params.set('include_expunged', 'true')
  for (const category of categories) params.append('category', category)
  if (bypassDefault) params.set('bypass_default', 'true')
  return `#/panda${params.size ? `?${params}` : ''}`
}
