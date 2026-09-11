import { useEffect, useRef, useState } from 'react'
import { updateSitemap } from './api'
import type { SitemapAction, SitemapStatus } from './api'

function date(value: string) { return new Date(value).toLocaleString() }

function errorLabel(error: string) {
  if (error === 'child_sitemaps_failed') return 'Some child sitemaps could not be collected. Retry to continue.'
  if (error === 'invalid_sitemap') return 'A sitemap could not be parsed.'
  if (error === 'request_failed') return 'A sitemap request failed.'
  if (error === 'panda_banned') return 'Panda temporarily restricted access.'
  if (/^upstream_http_\d+$/.test(error)) return `Panda returned HTTP ${error.slice('upstream_http_'.length)}.`
  return error
}

function stateLabel(status: SitemapStatus) {
  if (status.state === 'running') return status.retry_at && Date.parse(status.retry_at) > Date.now() ? 'Waiting to continue' : 'Collecting'
  if (status.state === 'completed') return 'Completed'
  if (status.state === 'incomplete') return 'Incomplete'
  if (status.state === 'cancelled') return 'Cancelled'
  return 'Idle'
}

export default function Sitemap({ status, available, onChanged }: {
  status: SitemapStatus
  available: boolean
  onChanged: (status: SitemapStatus) => void
}) {
  const [force, setForce] = useState(false)
  const [pending, setPending] = useState<SitemapAction | null>(null)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const mutation = useRef<AbortController | null>(null)
  useEffect(() => () => mutation.current?.abort(), [])
  const running = status.state === 'running'
  const disabled = !available || pending !== null
  const processed = status.children_completed + status.children_skipped + status.children_failed

  async function request(action: SitemapAction) {
    if (mutation.current || !available) return
    const controller = new AbortController()
    mutation.current = controller
    setPending(action)
    setError('')
    setNotice('')
    try {
      const result = await updateSitemap(action, force, controller.signal)
      if (!controller.signal.aborted) {
        onChanged(result)
        setNotice(action === 'cancel' ? 'Sitemap cancellation requested. Imported references and metadata work are retained.'
          : action === 'retry' ? 'Sitemap retry requested.' : 'Sitemap collection requested.')
      }
    } catch (error) {
      if (!controller.signal.aborted) setError((error as Error).message)
    } finally {
      mutation.current = null
      if (!controller.signal.aborted) setPending(null)
    }
  }

  return <section className="collector-panel" aria-labelledby="sitemap-title">
    <div className="collector-section-heading"><h2 id="sitemap-title">Panda sitemap</h2><span>{stateLabel(status)}{status.force && status.state !== 'idle' ? ' · Force refresh' : ''}</span></div>
    <p>Collect gallery references for automatic metadata fetching. Fresh discoveries take priority over sitemap backfill.</p>
    <form className="collector-sync" onSubmit={(event) => { event.preventDefault(); if (!disabled && !running) void request('start') }}>
      <label className="form-field">Collection mode
        <select className="text-input" value={force ? 'force' : 'conditional'} onChange={(event) => setForce(event.target.value === 'force')} disabled={disabled || running}>
          <option value="conditional">Skip unchanged sitemaps</option>
          <option value="force">Force refresh all sitemaps</option>
        </select>
      </label>
      <button className="button button-primary" type="submit" disabled={disabled || running}>{pending === 'start' ? 'Requesting…' : 'Collect sitemap'}</button>
      {running && <button className="button" type="button" disabled={disabled} onClick={() => void request('cancel')}>{pending === 'cancel' ? 'Cancelling…' : 'Cancel sitemap collection'}</button>}
      {(status.state === 'incomplete' || status.state === 'cancelled') && <button className="button" type="button" disabled={disabled} onClick={() => void request('retry')}>{pending === 'retry' ? 'Requesting…' : status.state === 'cancelled' ? 'Resume sitemap collection' : 'Retry sitemap collection'}</button>}
    </form>
    {status.state !== 'idle' && <>
      <p>{processed.toLocaleString()} of {status.children_total.toLocaleString()} child sitemaps processed{running && status.children_total === 0 ? ' · Reading index' : ''}</p>
      <progress className="sitemap-progress" aria-label="Sitemap collection progress" value={status.children_total ? processed : running ? undefined : 0} max={status.children_total || 1} />
      <dl className="collector-metrics">
        <div><dt>Child sitemaps imported</dt><dd>{status.children_completed.toLocaleString()}</dd></div>
        <div><dt>Unchanged sitemaps skipped</dt><dd>{status.children_skipped.toLocaleString()}</dd></div>
        <div><dt>Child sitemaps failed</dt><dd>{status.children_failed.toLocaleString()}</dd></div>
        <div><dt>Invalid gallery URLs skipped</dt><dd>{status.invalid_locations.toLocaleString()}</dd></div>
        <div><dt>Gallery references found</dt><dd>{status.references_found.toLocaleString()}</dd></div>
        <div><dt>Gallery references imported</dt><dd>{status.references_imported.toLocaleString()}</dd></div>
      </dl>
      {status.started_at && <p className="field-help">Started {date(status.started_at)}{status.finished_at ? ` · Finished ${date(status.finished_at)}` : ''}</p>}
      {running && status.retry_at && <p className="field-help">Continue after {date(status.retry_at)}.</p>}
    </>}
    <p className="field-help">Sitemap completion means reference import is complete. Metadata fetching continues independently; its backlog appears in Collector inventory.</p>
    {status.last_error && <p className="error-message">Sitemap collection: {errorLabel(status.last_error)}</p>}
    {notice && <p className="collector-notice" role="status">{notice}</p>}
    {error && <p className="error-message" role="alert">{error}</p>}
  </section>
}
