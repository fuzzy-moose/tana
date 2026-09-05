import { useEffect, useState } from 'react'
import { galleryLink, readerSpreadLink } from '../navigation'
import type { Gallery } from './api'
import { useGallery } from './useGallery'
import { useReaderImages } from './useReaderImages'
import { previousSpread, spreadPages } from './spreads'
import ReaderProgress from './ReaderProgress'
import './Reader.css'

export default function Reader({ id, initialPage, initialLastPage }: { id: number, initialPage: number, initialLastPage?: number }) {
  const { gallery, error, retry } = useGallery(id)
  useEffect(() => {
    const escape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') window.location.hash = galleryLink(id)
    }
    window.addEventListener('keydown', escape)
    return () => window.removeEventListener('keydown', escape)
  }, [id])

  if (gallery && gallery.page_count > 0) return <ReadingGallery gallery={gallery} initialPage={initialPage} initialLastPage={initialLastPage} />
  return <main className="reader reader-message">
    <a className="button" href={galleryLink(id)}>← Gallery detail</a>
    {error ? <div role="alert"><p>{error}</p><button className="button" onClick={retry}>Retry</button></div>
      : <p role="status">{gallery ? 'This gallery has no pages to read.' : 'Loading gallery…'}</p>}
  </main>
}

function ReadingGallery({ gallery, initialPage, initialLastPage }: { gallery: Gallery, initialPage: number, initialLastPage?: number }) {
  const total = gallery.page_count
  const [start, setStart] = useState(Math.max(1, Math.min(initialPage, total)))
  const [controlsVisible, setControlsVisible] = useState(false)
  // Keep the selected pairing when reversing through the cover boundary.
  // A one-page turn starts a fresh pairing; normal turns retain spread starts.
  const [boundaries, setBoundaries] = useState(() => new Set([1, start, total + 1, ...(initialLastPage === start ? [start + 1] : [])]))
  const { images, retry } = useReaderImages(gallery.id, start, total)
  const shape = (page: number) => images.get(page)?.shape
  const pages = boundaries.has(start + 1) ? [start] : spreadPages(start, total, shape)
  const ready = images.has(start) && (start === 1 || start === total || shape(start) === 'wide' || images.has(start + 1))
  const previousReady = start <= 2 || (images.has(start - 1) && (start === 3 || images.has(start - 2)))
  const lastPage = pages[pages.length - 1]
  const atEnd = lastPage === total

  useEffect(() => {
    if (!ready) return
    const hash = readerSpreadLink(gallery.id, start, lastPage)
    // Replacing the entry preserves Back/Forward without triggering a reader remount.
    if (window.location.hash !== hash) window.history.replaceState(window.history.state, '', hash)
  }, [gallery.id, start, lastPage, ready])

  function selectPage(page: number) {
    if (page === start) return
    setStart(page)
    setBoundaries(new Set([1, page, total + 1]))
  }

  function move(direction: 1 | -1, single = false) {
    if (single) {
      selectPage(Math.max(1, Math.min(total, start + direction)))
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
      const key = event.key.toLowerCase()
      if (key === 'arrowleft' || key === 'arrowright' || key === 'a' || key === 'd') {
        event.preventDefault()
        move(key === 'arrowleft' || key === 'a' ? 1 : -1, event.shiftKey)
      }
    }
    window.addEventListener('keydown', navigate)
    return () => window.removeEventListener('keydown', navigate)
  })

  return <main className="reader" aria-label={`Reading ${gallery.title}`} data-controls-visible={controlsVisible}>
    <header className="reader-header" id="reader-header">
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
      <button className="reader-toggle" aria-label="Toggle reader controls" aria-expanded={controlsVisible} aria-controls="reader-header reader-footer" tabIndex={-1} onClick={() => setControlsVisible((visible) => !visible)} />
    </div>
    <div className="reader-bottom">
      <footer className="reader-footer" id="reader-footer">
        <div className="reader-controls">
          <button className="button" title="Left arrow" disabled={!ready || atEnd} onClick={() => move(1)}>← Next</button>
          <button className="button reader-single-turn" title="Shift + left arrow" disabled={start === total} onClick={() => move(1, true)}>← 1 page</button>
          <p role="status" className="reader-position">{ready ? `Page ${pages.join('–')} of ${total}` : `Page ${start} of ${total}`}{ready && atEnd && <span>End of gallery</span>}</p>
          <button className="button reader-single-turn" title="Shift + right arrow" disabled={start === 1} onClick={() => move(-1, true)}>1 page →</button>
          <button className="button" title="Right arrow" disabled={start === 1 || !previousReady} onClick={() => move(-1)}>Previous →</button>
        </div>
        <p className="reader-help">
          <span className="reader-help-desktop">Click either side or use arrow keys / A / D · Hold Shift to turn one page · Esc returns to gallery</span>
          <span className="reader-help-touch">Tap sides to turn pages · Tap center for controls</span>
        </p>
      </footer>
      <ReaderProgress page={start} lastPage={ready ? lastPage : start} total={total} onSelect={selectPage} />
    </div>
  </main>
}
