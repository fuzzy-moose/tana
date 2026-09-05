import { useEffect, useState } from 'react'
import { galleryLink, listingLink } from '../navigation'
import { listGalleries } from './api'
import type { GalleryListing } from './api'
import GalleryImage from './GalleryImage'
import './Galleries.css'

export default function Galleries({ search, page }: { search: string, page: number }) {
  const [result, setResult] = useState<GalleryListing | null>(null)
  const [query, setQuery] = useState(search)
  const [error, setError] = useState('')
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function refresh() {
      try {
        const next = await listGalleries(search, page, controller.signal)
        if (!controller.signal.aborted) { setResult(next); setError('') }
      } catch (error) {
        if (!controller.signal.aborted) setError((error as Error).message)
      } finally {
        // A scan can continue after leaving Libraries; new galleries appear here.
        if (!controller.signal.aborted) timer = setTimeout(refresh, 5000)
      }
    }
    void refresh()
    return () => { controller.abort(); clearTimeout(timer) }
  }, [search, page, attempt])

  return (
    <section aria-labelledby="galleries-title">
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
      {!result && !error && <p className="gallery-meta" role="status">Loading galleries…</p>}
      {result && <>
        <div className="gallery-meta"><span>{result.total} {result.total === 1 ? 'gallery' : 'galleries'}</span><span>Title · A–Z</span></div>
        {result.items.length === 0 ? <div className="library-placeholder">
          <h2>{search ? 'No matching galleries.' : 'Your next read starts here.'}</h2>
          <p>{search ? 'Try a different title.' : 'Add a library and scan it to discover your comics and manga.'}</p>
          <a className="button" href={search ? '#/' : '#/libraries'}>{search ? 'Clear search' : 'Manage libraries'}</a>
        </div> : <ul className="gallery-grid" aria-label="Galleries">
          {result.items.map((gallery) => <li key={gallery.id}>
            <a className="gallery-card" href={galleryLink(gallery.id)}>
              <div className="gallery-cover">{gallery.page_count > 0
                ? <GalleryImage id={gallery.id} page={1} alt="" />
                : <span className="image-placeholder">No pages</span>}</div>
              <h2>{gallery.title}</h2>
              <p>{gallery.page_count} {gallery.page_count === 1 ? 'page' : 'pages'}</p>
            </a>
          </li>)}
        </ul>}
        {result.total > result.page_size && <nav className="pagination" aria-label="Gallery pages">
          {result.page > 1 ? <a className="button" href={listingLink(search, result.page - 1)}>Previous</a> : <button className="button" disabled>Previous</button>}
          <span>Page {result.page} of {Math.ceil(result.total / result.page_size)}</span>
          {result.page * result.page_size < result.total ? <a className="button" href={listingLink(search, result.page + 1)}>Next</a> : <button className="button" disabled>Next</button>}
        </nav>}
      </>}
    </section>
  )
}
