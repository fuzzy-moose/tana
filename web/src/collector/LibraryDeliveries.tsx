import { deliveryActive, deliveryItemState } from './libraryDeliveries'
import type { useLibraryDeliveries } from './useLibraryDeliveries'

export default function LibraryDeliveries({ deliveries, libraryID, onLibraryChange, canStart, completedCount }: {
  deliveries: ReturnType<typeof useLibraryDeliveries>
  libraryID: number
  onLibraryChange: (id: number) => void
  canStart: boolean
  completedCount: number
}) {
  return <section className="library-deliveries" aria-labelledby="library-deliveries-title">
    <div className="collector-section-heading">
      <h3 id="library-deliveries-title">Add downloads to a library</h3>
      <button className="button" type="button" disabled={deliveries.pending || deliveries.checking} onClick={deliveries.refresh}>Refresh deliveries</button>
    </div>
    <p>Transfer completed archives into a library, one at a time. Each collector copy is deleted after successful import. Delivery continues when you close this page. Successful batches disappear automatically.</p>
    <div className="collector-sync">
      <label className="form-field">Destination library
        <select className="text-input" value={libraryID || ''} onChange={(event) => onLibraryChange(Number(event.target.value))}
          disabled={deliveries.pending || !!deliveries.active || !deliveries.librariesLoaded || !!deliveries.libraryError}>
          <option value="">Choose a library</option>
          {deliveries.libraries.map((library) => <option key={library.id} value={library.id}>{library.name}{library.availability === 'unavailable' ? ' (unavailable)' : ''}</option>)}
        </select>
      </label>
      <button className="button button-primary" type="button" disabled={!canStart || completedCount === 0}
        onClick={() => void deliveries.start({ library_id: libraryID, all: true })}>Add all</button>
    </div>
    <p className="field-help">Add all includes every completed download when clicked, across all pages and filters.</p>
    {deliveries.librariesLoaded && deliveries.libraries.length === 0 && <p className="field-help"><a href="#/libraries">Register a library</a> to add downloads.</p>}
    {deliveries.active && <p className="collector-notice" role="status">One delivery batch at a time. Finish or stop the active batch to start another.</p>}
    {deliveries.libraryError && <p className="error-message" role="alert">{deliveries.libraryError}</p>}
    {deliveries.requestError && <p className="error-message" role="alert">{deliveries.requestError}</p>}
    {deliveries.error && <p className="error-message" role="alert">{deliveries.error} Delivery progress may be stale.</p>}
    {!deliveries.loaded && !deliveries.error && <p className="field-help">Loading deliveries…</p>}
    {deliveries.batches.map((batch) => {
      const active = deliveryActive(batch)
      const done = batch.items.filter((item) => ['completed', 'skipped', 'failed', 'cleanup_pending'].includes(item.state)).length
      const added = batch.items.filter((item) => ['completed', 'cleanup_pending'].includes(item.state)).length
      const failed = batch.items.filter((item) => item.state === 'failed').length
      const cleanup = batch.items.filter((item) => item.state === 'cleanup_pending').length
      const cleanupExhausted = batch.items.some((item) => item.state === 'cleanup_pending' && item.cleanup_attempts >= 3)
      const skipped = batch.items.filter((item) => item.state === 'skipped').length
      const disabled = deliveries.disabled
      const retryDisabled = disabled || !!deliveries.active
      const library = deliveries.libraries.find((library) => library.id === batch.library_id)
      const current = batch.items.find((item) => ['transferring', 'transferred', 'saved', 'importing'].includes(item.state))
      const state = { running: 'Delivering', paused: 'Paused', stopped: 'Stopped', completed: 'Completed', completed_with_errors: 'Needs attention' }[batch.state]
      return <article className="library-delivery" key={batch.id} aria-label={`Delivery ${batch.id}`}>
        <div className="collector-section-heading">
          <h4>Delivery {batch.id} · {library?.name ?? `Library ${batch.library_id}`}</h4>
          <span className="collector-badge">{batch.stop_requested && batch.state === 'running' ? 'Stopping after current archive' : state}</span>
        </div>
        <p>{done} of {batch.items.length} processed · {added} added · {skipped} already in library · {failed} failed{cleanup > 0 && ` · ${cleanup} awaiting collector cleanup`}</p>
        <progress className="sitemap-progress" aria-label={`Delivery ${batch.id} progress`} value={done} max={batch.items.length || 1} />
        {current && <p className="field-help">Gallery {current.gallery_id} · {deliveryItemState(current)}</p>}
        {batch.error && <p className="error-message">{batch.error}</p>}
        {batch.state === 'paused' && <p className="field-help">Progress is saved. Resume explicitly when the destination is available.</p>}
        {batch.state === 'stopped' && <p className="field-help">Remaining archives were left on the collector. Retry remaining to continue this batch.</p>}
        <div className="button-group library-delivery-actions">
          {batch.state === 'paused' && <button className="button" disabled={disabled} onClick={() => void deliveries.change(batch.id, 'resume')}>Resume</button>}
          {active && <button className="button" disabled={disabled || batch.state === 'running' && batch.stop_requested} onClick={() => void deliveries.change(batch.id, 'stop')}>{batch.state === 'paused' ? 'Stop batch' : 'Stop after current archive'}</button>}
          {!active && (failed > 0 || batch.state === 'stopped' && batch.items.some((item) => ['queued', 'transferring', 'transferred', 'saved', 'importing'].includes(item.state))) && <button className="button" disabled={retryDisabled} onClick={() => void deliveries.change(batch.id, 'retry')}>{batch.state === 'stopped' ? 'Retry remaining' : 'Retry failed'}</button>}
          {cleanupExhausted && <button className="button" disabled={disabled} onClick={() => void deliveries.change(batch.id, 'retry-cleanup')}>Retry cleanup</button>}
        </div>
        <details className="library-delivery-items">
          <summary>Gallery results ({batch.items.length})</summary>
          <ul className="collector-errors">{batch.items.map((item) => <li key={item.gallery_id}>
            <strong>Gallery {item.gallery_id} · {deliveryItemState(item)}</strong>
            {item.error && item.state !== 'skipped' && <span>{item.error}</span>}
          </li>)}</ul>
        </details>
      </article>
    })}
  </section>
}
