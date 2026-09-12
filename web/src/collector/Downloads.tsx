import { useState } from 'react'
import { downloadFailure, downloadFileURL, downloadPageSize, downloadState } from './downloads'
import type { DownloadFilter } from './downloads'
import { useDownloads } from './useDownloads'

const filters: { state: DownloadFilter, label: string }[] = [
  { state: '', label: 'All' },
  { state: 'running', label: 'Active' },
  { state: 'queued', label: 'Pending' },
  { state: 'completed', label: 'Completed' },
  { state: 'failed', label: 'Failed' },
  { state: 'cancelled', label: 'Cancelled' },
  { state: 'deleting', label: 'Deleting' },
]

function date(value: string) { return new Date(value).toLocaleString() }
function size(bytes: number) {
  if (bytes < 1024) return `${bytes.toLocaleString()} B`
  if (bytes < 1024 ** 2) return `${(bytes / 1024).toFixed(1)} KiB`
  if (bytes < 1024 ** 3) return `${(bytes / 1024 ** 2).toFixed(1)} MiB`
  return `${(bytes / 1024 ** 3).toFixed(1)} GiB`
}

export default function Downloads({ available, refreshKey }: { available: boolean, refreshKey: number }) {
  const downloads = useDownloads(available, refreshKey)
  const [url, setURL] = useState('')
  const [deleting, setDeleting] = useState<number | null>(null)
  const actionsDisabled = downloads.disabled || downloads.stale || downloads.checking || !downloads.loaded
  const filterLabel = filters.find((filter) => filter.state === downloads.filter)!.label
  const total = downloads.counts ? Object.values(downloads.counts).reduce((sum, count) => sum + count, 0) : undefined

  return (
    <section className="collector-panel" aria-labelledby="downloads-title">
      <div className="collector-section-heading">
        <h2 id="downloads-title">Downloads</h2>
        <button className="button" type="button" disabled={!available || downloads.checking || downloads.pending} onClick={downloads.refresh}>Refresh downloads</button>
      </div>
      <p>Download original Panda archives to the collector. Save ZIP copies an archive to your device; it remains on the collector until deleted. Archives are not added to a Tana library automatically.</p>
      <form className="collector-sync" onSubmit={async (event) => {
        event.preventDefault()
        if (await downloads.submit(url)) setURL('')
      }}>
        <label className="form-field">Panda gallery URL
          <input className="text-input" type="url" required value={url} onChange={(event) => setURL(event.target.value)} disabled={downloads.disabled} placeholder="Paste a gallery URL" autoComplete="off" />
        </label>
        <button className="button button-primary" type="submit" disabled={downloads.disabled || !url.trim()}>{downloads.pending ? 'Requesting…' : 'Add download'}</button>
      </form>
      <p className="field-help">Submitting the same gallery reuses its existing download. Failed or cancelled jobs can be retried below.</p>
      <div className="download-filters" role="group" aria-label="Filter downloads">
        {filters.map(({ state, label }) => <button className="button download-filter" key={state} type="button"
          aria-pressed={downloads.filter === state} disabled={downloads.disabled}
          onClick={() => { downloads.selectFilter(state); setDeleting(null) }}>
          <span>{label}</span>{' '}
          <strong>{(state ? downloads.counts?.[state] : total)?.toLocaleString() ?? '—'}</strong>
        </button>)}
      </div>
      {downloads.filter === 'queued' && <p className="field-help">Pending downloads are queued or waiting for an automatic retry or Panda cooldown.</p>}
      {downloads.notice && <p className="collector-notice" role="status">{downloads.notice}</p>}
      {downloads.requestError && <p className="error-message" role="alert">{downloads.requestError}</p>}
      {downloads.error && <p className="error-message" role="alert">{downloads.error}</p>}
      {downloads.stale && <p className="collector-stale" role="status">Download status is stale. Actions will be available when the collector reconnects and the list refreshes.</p>}
      {!downloads.loaded && !downloads.error && <p className="field-help">{available ? 'Loading downloads…' : 'Downloads are unavailable while the collector is disconnected.'}</p>}
      {downloads.loaded && downloads.jobs.length === 0 && <p className="download-empty" role="status">{downloads.filter
        ? `No ${filterLabel.toLowerCase()} downloads.`
        : 'No downloads yet. Paste a Panda gallery URL to get started.'}</p>}
      {downloads.jobs.length > 0 && <div className="collector-table-scroll">
        <table className="collector-table collector-downloads">
          <caption className="collector-table-caption">{filterLabel} downloads · newest first</caption>
          <thead><tr><th scope="col">Gallery</th><th scope="col">State</th><th scope="col">Archive size</th><th scope="col">Dates</th><th scope="col">Actions</th></tr></thead>
          <tbody>{downloads.jobs.map((job) => <tr key={job.gallery_id}>
            <th scope="row">Gallery {job.gallery_id}</th>
            <td><span className="collector-state download-state" data-state={job.state}>{downloadState(job)}</span>
              {job.error && <span className="collector-secondary">{downloadFailure(job.error)}</span>}
              {job.failures > 0 && <span className="collector-secondary">{job.failures} failed {job.failures === 1 ? 'attempt' : 'attempts'}</span>}
              {job.retry_at && <span className="collector-secondary">Retry after {date(job.retry_at)}</span>}
            </td>
            <td>{job.state === 'completed' ? size(job.size_bytes) : '—'}</td>
            <td><span>Added {date(job.created_at)}</span><span className="collector-secondary">Updated {date(job.updated_at)}</span></td>
            <td>
              <div className="button-group">
                {job.state === 'completed' && (actionsDisabled
                  ? <button className="button" disabled>Save ZIP</button>
                  : <a className="button" href={downloadFileURL(job.gallery_id)} download>Save ZIP</a>)}
                {(job.state === 'queued' || job.state === 'running') && <button className="button" disabled={actionsDisabled} onClick={() => void downloads.change(job.gallery_id, 'cancel')}>Cancel</button>}
                {(job.state === 'failed' || job.state === 'cancelled') && <button className="button" disabled={actionsDisabled} onClick={() => void downloads.change(job.gallery_id, 'retry')}>Retry</button>}
                {['completed', 'failed', 'cancelled', 'deleting'].includes(job.state) && <button className="button" disabled={actionsDisabled} onClick={() => setDeleting(job.gallery_id)}>Delete</button>}
              </div>
              {deleting === job.gallery_id && <div className="download-delete" role="group" aria-label={`Delete download for gallery ${job.gallery_id}`}>
                <p>Delete this download and any archive retained on the collector?</p>
                <div className="button-group">
                  <button className="button" disabled={actionsDisabled} onClick={async () => { if (await downloads.change(job.gallery_id, 'delete')) setDeleting(null) }}>Delete download</button>
                  <button className="button" disabled={downloads.pending} onClick={() => setDeleting(null)}>Keep download</button>
                </div>
              </div>}
            </td>
          </tr>)}</tbody>
        </table>
      </div>}
      {(downloads.offset > 0 || downloads.hasNext) && <nav className="download-pagination" aria-label="Download pages">
        <button className="button" disabled={actionsDisabled || downloads.checking || downloads.offset === 0} onClick={downloads.previous}>Previous downloads</button>
        <span>Page {downloads.offset / downloadPageSize + 1}</span>
        <button className="button" disabled={actionsDisabled || downloads.checking || !downloads.hasNext} onClick={downloads.next}>Next downloads</button>
      </nav>}
    </section>
  )
}
