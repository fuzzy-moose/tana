import { useCallback, useEffect, useState } from 'react'
import type { CSSProperties } from 'react'
import GallerySearch from '../galleries/GallerySearch'
import { replaceListingPage, useGalleryLayout } from '../galleries/useGalleryLayout'
import { pandaListingLink } from '../navigation'
import Pagination from '../pagination/Pagination'
import { completePandaSearch, listPandaCatalog } from './api'
import type { PandaCatalogResult } from './api'
import PandaCard from './PandaCard'
import { usePandaCollection } from './usePandaCollection'
import '../galleries/Galleries.css'
import './PandaCatalog.css'

interface PandaCatalogProps {
  search: string
  page: number
  includeExpunged: boolean
}

export default function PandaCatalog({ search, page, includeExpunged }: PandaCatalogProps) {
  const [result, setResult] = useState<PandaCatalogResult | null>(null)
  const [requestState, setRequestState] = useState<{ key: string, error: string } | null>(null)
  const [attempt, setAttempt] = useState(0)
  const collection = usePandaCollection()
  const pageHref = useCallback((query: string, number = 1) => pandaListingLink(query, number, includeExpunged), [includeExpunged])
  const { viewportRef, cardRef, pageSize, cardHeight } = useGalleryLayout(search, page, pageHref)
  const requestKey = JSON.stringify([search, page, pageSize, includeExpunged, attempt])
  const loading = requestState?.key !== requestKey
  const error = loading ? '' : requestState.error

  useEffect(() => {
    if (!pageSize) return
    const controller = new AbortController()
    void listPandaCatalog(search, page, pageSize, includeExpunged, controller.signal).then((next) => {
      if (controller.signal.aborted) return
      setResult(next)
      setRequestState({ key: requestKey, error: '' })
      if (next.page !== page) replaceListingPage(search, next.page, pageHref)
    }).catch((error: Error) => {
      if (!controller.signal.aborted) setRequestState({ key: requestKey, error: error.message })
    })
    return () => controller.abort()
  }, [search, page, pageSize, includeExpunged, requestKey, pageHref])

  useEffect(() => {
    if (!result || loading) return
    const navigate = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.isComposing || event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) return
      if (event.target instanceof HTMLElement && (event.target.isContentEditable || event.target.closest('input, textarea, select, [contenteditable]:not([contenteditable="false"])'))) return
      const direction = event.key === 'ArrowLeft' || event.key === 'a' ? -1 : event.key === 'ArrowRight' || event.key === 'd' ? 1 : 0
      if (!direction) return
      event.preventDefault()
      const next = result.page + direction
      if (next >= 1 && next <= result.total_pages) window.location.hash = pageHref(search, next)
    }
    window.addEventListener('keydown', navigate)
    return () => window.removeEventListener('keydown', navigate)
  }, [result, loading, pageHref, search])

  function retry() {
    setAttempt((value) => value + 1)
    collection.retry()
  }

  const unavailable = error || collection.error
  const feed = collection.feed

  return <section className="gallery-listing panda-catalog" aria-labelledby="panda-title" style={{ '--gallery-card-height': `${cardHeight}px` } as CSSProperties}>
    <div className="gallery-toolbar">
      <div className="page-heading">
        <p className="eyebrow">Collected uploads</p>
        <h1 id="panda-title">Panda</h1>
        <p>Browse collected galleries. Open on Panda.</p>
      </div>
      <GallerySearch key={search} search={search} completeSearch={completePandaSearch} searchHref={pageHref} />
    </div>

    <div className="panda-controls">
      <label><input type="checkbox" checked={includeExpunged} onChange={(event) => { window.location.hash = pandaListingLink(search, 1, event.target.checked) }} />Include expunged</label>
      <div className="button-group">
        <button className="button" disabled={loading} onClick={() => setAttempt((value) => value + 1)}>{loading && result ? 'Reloading…' : 'Reload results'}</button>
        <button className="button" disabled={collection.pending || feed?.capture_active} onClick={() => void collection.capture()}>{collection.pending || feed?.capture_active ? 'Capturing feed…' : 'Refresh feed'}</button>
      </div>
    </div>

    <div className="panda-collection" aria-label="Panda collection status">
      <p>{feed ? <>
        {feed.capture_active ? 'Feed capture in progress.' : feed.last_captured_at ? <>Last feed captured <time dateTime={feed.last_captured_at}>{new Date(feed.last_captured_at).toLocaleString()}</time>.</> : 'No feed captured yet.'}
        {' '}{feed.processing_pending > 0 ? `${feed.processing_pending} feeds awaiting processing.` : 'Feed processing complete.'}
      </> : 'Checking feed status…'}</p>
      <p>{collection.inventory ? `${collection.inventory.metadata_pending.toLocaleString()} metadata pending · ${collection.inventory.metadata_failed.toLocaleString()} failed.` : 'Checking metadata collection…'}{' '}Search covers collected metadata from all discovery paths.</p>
      <p>{feed?.possible_gaps ? `${feed.possible_gaps} possible feed gaps.` : feed?.continuity === 'overlap' ? 'Latest feed overlaps earlier captures.' : 'Feed continuity unknown.'}{' '}<a href="#/collector">Collection details</a></p>
      {feed?.last_capture_error && <p className="error-message">Feed capture: {feed.last_capture_error}</p>}
      {feed?.processing_error && <p className="error-message">Feed processing: {feed.processing_error}</p>}
      {collection.requestError && <p className="error-message" role="alert">{collection.requestError}</p>}
    </div>

    <div className="panda-feedback" aria-live="polite">
      {unavailable ? <div className="panda-error" role="alert"><p>{result ? 'Results are stale. Showing previously loaded galleries.' : 'Panda catalog unavailable.'} {unavailable}</p><button className="button" onClick={retry}>Retry</button></div>
        : <p>{loading && result ? 'Loading results…' : 'Refresh feed collects uploads. Reload results to see newly collected galleries.'}</p>}
    </div>

    <div className="gallery-meta"><span>{result ? `${result.total.toLocaleString()} ${result.total === 1 ? 'gallery' : 'galleries'}` : '\u00a0'}</span><span>Upload date · Newest first</span></div>
    <div className="gallery-viewport" ref={viewportRef} aria-busy={loading}>
      <div className="gallery-grid gallery-sizing" aria-hidden="true"><div className="gallery-card panda-card" ref={cardRef}><div className="gallery-cover" /><div className="gallery-title"><h2 /></div><p>0 pages · 12/12/2026</p></div></div>
      {!result && loading && !unavailable && <p className="gallery-loading" role="status">Loading Panda galleries…</p>}
      {result && (result.items.length === 0 ? <div className="library-placeholder"><h2>{search ? 'No matching Panda galleries.' : 'No collected Panda galleries yet.'}</h2><p>{search ? 'Try different titles or tags.' : 'Refresh the feed and wait for metadata collection, then reload results.'}</p>{search && <a className="button" href={pageHref('')}>Clear search</a>}</div>
        : <ul className="gallery-grid" aria-label="Panda galleries">{result.items.map((gallery) => <li key={gallery.gallery_id}><PandaCard gallery={gallery} /></li>)}</ul>)}
    </div>
    <div className="gallery-pagination">{result && <Pagination page={result.page} totalPages={result.total_pages} pageHref={(number) => pageHref(search, number)} adjacentCount={1} label="Panda gallery pages" />}</div>
  </section>
}
