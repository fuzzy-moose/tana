import { useEffect, useState } from 'react'
import type { CSSProperties } from 'react'
import { listingLink } from '../navigation'
import Pagination from '../pagination/Pagination'
import { listGalleries } from './api'
import type { GalleryListing } from './api'
import GalleryCard from './GalleryCard'
import { replaceListingPage, useGalleryLayout } from './useGalleryLayout'
import './Galleries.css'

export default function Galleries({ search, page }: { search: string, page: number }) {
  const [result, setResult] = useState<GalleryListing | null>(null)
  const [query, setQuery] = useState(search)
  const [error, setError] = useState('')
  const [attempt, setAttempt] = useState(0)
  const { viewportRef, cardRef, pageSize, cardHeight } = useGalleryLayout(search, page)
  const current = result?.page_size === pageSize && result.page === page ? result : null

  useEffect(() => {
    if (!pageSize) return
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function refresh() {
      try {
        const next = await listGalleries(search, page, pageSize, controller.signal)
        if (!controller.signal.aborted) {
          setResult(next)
          setError('')
          if (next.page !== page) replaceListingPage(search, next.page)
        }
      } catch (error) {
        if (!controller.signal.aborted) setError((error as Error).message)
      } finally {
        // A scan can continue after leaving Libraries; new galleries appear here.
        if (!controller.signal.aborted) timer = setTimeout(refresh, 5000)
      }
    }
    void refresh()
    return () => { controller.abort(); clearTimeout(timer) }
  }, [search, page, pageSize, attempt])

  useEffect(() => {
    if (!current) return
    const navigate = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.isComposing || event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) return
      if (event.target instanceof HTMLElement && (event.target.isContentEditable || event.target.closest('input, textarea, select, [contenteditable]:not([contenteditable="false"])'))) return
      const direction = event.key === 'ArrowLeft' || event.key === 'a' ? -1
        : event.key === 'ArrowRight' || event.key === 'd' ? 1 : 0
      if (!direction) return
      event.preventDefault()
      const nextPage = current.page + direction
      if (nextPage >= 1 && nextPage <= Math.ceil(current.total / current.page_size)) window.location.hash = listingLink(search, nextPage)
    }
    window.addEventListener('keydown', navigate)
    return () => window.removeEventListener('keydown', navigate)
  }, [current, search])

  return (
    <section className="gallery-listing" aria-labelledby="galleries-title" style={{ '--gallery-card-height': `${cardHeight}px` } as CSSProperties}>
      <div className="gallery-toolbar">
        <div className="page-heading">
          <p className="eyebrow">Your collection</p>
          <h1 id="galleries-title">Galleries</h1>
          <p>A little space to get lost in a story.</p>
        </div>
        <form className="gallery-search" role="search" onSubmit={(event) => { event.preventDefault(); window.location.hash = listingLink(query.trim()) }}>
          <label className="form-field" htmlFor="gallery-search">Search titles</label>
          <div className="button-group">
            <input id="gallery-search" className="text-input" type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Find a gallery…" />
            <button className="button" type="submit">Search</button>
          </div>
        </form>
      </div>
      {error && <div className="error-banner" role="alert"><p>{error}</p><button className="button" onClick={() => setAttempt((n) => n + 1)}>Retry</button></div>}
      <div className="gallery-meta">
        <span>{result ? `${result.total} ${result.total === 1 ? 'gallery' : 'galleries'}` : '\u00a0'}</span><span>Title · A–Z</span>
      </div>
      <div className="gallery-viewport" ref={viewportRef} aria-busy={!current && !error}>
        <div className="gallery-grid gallery-sizing" aria-hidden="true">
          <div className="gallery-card" ref={cardRef}><div className="gallery-cover" /><div className="gallery-title"><h2 /></div><p>0 pages</p></div>
        </div>
        {!current && !error && <p className="gallery-loading" role="status">Loading galleries…</p>}
        {current && (current.items.length === 0 ? <div className="library-placeholder">
          <h2>{search ? 'No matching galleries.' : 'Your next read starts here.'}</h2>
          <p>{search ? 'Try a different title.' : 'Add a library and scan it to discover your comics and manga.'}</p>
          <a className="button" href={search ? '#/' : '#/libraries'}>{search ? 'Clear search' : 'Manage libraries'}</a>
        </div> : <ul className="gallery-grid" aria-label="Galleries">
          {current.items.map((gallery) => <li key={gallery.id}><GalleryCard gallery={gallery} /></li>)}
        </ul>)}
      </div>
      <div className="gallery-pagination">
        {result && <Pagination page={current?.page ?? page} totalPages={Math.ceil(result.total / (pageSize || 1))} pageHref={(number) => listingLink(search, number)} adjacentCount={1} label="Gallery pages" />}
      </div>
    </section>
  )
}
