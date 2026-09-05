import { useEffect, useState } from 'react'
import { galleryLink } from '../navigation'
import type { Gallery } from './api'
import { useGallery } from './useGallery'
import { useReaderImages } from './useReaderImages'
import { previousSpread, spreadPages } from './spreads'
import './Reader.css'

export default function Reader({ id, initialPage }: { id: string, initialPage: number }) {
  const { gallery, error, retry } = useGallery(id)
  useEffect(() => {
    const escape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') window.location.hash = galleryLink(id)
    }
    window.addEventListener('keydown', escape)
    return () => window.removeEventListener('keydown', escape)
  }, [id])

  if (gallery && gallery.page_count > 0) return <ReadingGallery gallery={gallery} initialPage={initialPage} />
  return <main className="reader reader-message">
    <a className="button" href={galleryLink(id)}>← Gallery detail</a>
    {error ? <div role="alert"><p>{error}</p><button className="button" onClick={retry}>Retry</button></div>
      : <p role="status">{gallery ? 'This gallery has no pages to read.' : 'Loading gallery…'}</p>}
  </main>
}

function ReadingGallery({ gallery, initialPage }: { gallery: Gallery, initialPage: number }) {
  const total = gallery.page_count
  const [start, setStart] = useState(Math.max(1, Math.min(initialPage, total)))
  // Keep the selected pairing when reversing through the cover boundary.
  // A one-page turn starts a fresh pairing; normal turns retain spread starts.
  const [boundaries, setBoundaries] = useState(() => new Set([1, start, total + 1]))
  const { images, retry } = useReaderImages(gallery.id, start, total)
  const shape = (page: number) => images.get(page)?.shape
  const pages = boundaries.has(start + 1) ? [start] : spreadPages(start, total, shape)
  const ready = images.has(start) && (start === 1 || start === total || shape(start) === 'wide' || images.has(start + 1))
  const previousReady = start <= 2 || (images.has(start - 1) && (start === 3 || images.has(start - 2)))
  const atEnd = pages[pages.length - 1] === total

  function move(direction: 1 | -1, single = false) {
    if (single) {
      const next = Math.max(1, Math.min(total, start + direction))
      if (next !== start) {
        setStart(next)
        setBoundaries(new Set([1, next, total + 1]))
      }
    } else if (direction === 1 && ready && !atEnd) {
      const next = start + pages.length
      setStart(next)
      setBoundaries((current) => new Set([...current, next]))
    } else if (direction === -1 && previousReady) {
      const next = boundaries.has(start - 1) ? start - 1 : previousSpread(start, shape)
      setStart(next)
      setBoundaries((current) => new Set([...current, next]))
    }
  }

  useEffect(() => {
    const navigate = (event: KeyboardEvent) => {
      if (event.altKey || event.ctrlKey || event.metaKey || (event.target instanceof HTMLElement && event.target.matches('input, textarea, select, [contenteditable="true"]'))) return
      if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
        event.preventDefault()
        move(event.key === 'ArrowLeft' ? 1 : -1, event.shiftKey)
      }
    }
    window.addEventListener('keydown', navigate)
    return () => window.removeEventListener('keydown', navigate)
  })

  return <main className="reader" aria-label={`Reading ${gallery.title}`}>
    <header className="reader-header">
      <a className="button" href={galleryLink(gallery.id)}>← Gallery detail</a>
      <h1>{gallery.title}</h1>
      <span>Right to left</span>
    </header>
    <div className="reader-stage" aria-label="Reading spread" aria-busy={!ready}>
      {ready ? <div className={`reader-spread ${pages.length === 1 ? 'reader-single' : ''}`}>
        {pages.map((page) => <div className="reader-page" key={page}>
          {images.get(page)?.shape === 'error' ? <div className="reader-page-error" role="group" aria-label={`Page ${page} unavailable`}>
            <p>Page {page} unavailable</p>
            <button className="button" onClick={() => retry(page)}>Retry page {page}</button>
          </div> : <img src={images.get(page)?.url} alt={`Page ${page}`} draggable={false} />}
        </div>)}
      </div> : <p className="reader-loading" role="status">Loading pages…</p>}
      <button className="reader-side reader-side-left" aria-label="Next spread" tabIndex={-1} disabled={!ready || atEnd} onClick={(event) => move(1, event.shiftKey)} />
      <button className="reader-side reader-side-right" aria-label="Previous spread" tabIndex={-1} disabled={start === 1 || !previousReady} onClick={(event) => move(-1, event.shiftKey)} />
    </div>
    <footer className="reader-footer">
      <div className="reader-controls">
        <button className="button" title="Left arrow" disabled={!ready || atEnd} onClick={() => move(1)}>← Next</button>
        <button className="button reader-single-turn" title="Shift + left arrow" disabled={start === total} onClick={() => move(1, true)}>← 1 page</button>
        <p role="status" className="reader-position">{ready ? `Page ${pages.join('–')} of ${total}` : `Page ${start} of ${total}`}{ready && atEnd && <span>End of gallery</span>}</p>
        <button className="button reader-single-turn" title="Shift + right arrow" disabled={start === 1} onClick={() => move(-1, true)}>1 page →</button>
        <button className="button" title="Right arrow" disabled={start === 1 || !previousReady} onClick={() => move(-1)}>Previous →</button>
      </div>
      <p className="reader-help">Click either side or use arrow keys · Shift + arrow turns one page · Esc returns to gallery</p>
    </footer>
  </main>
}
