import { useState } from 'react'
import type { FavoriteCategory } from './api'
import { useCollector } from './useCollector'
import Downloads from './Downloads'
import FavoriteDownloads from './FavoriteDownloads'
import './Collector.css'

function date(value?: string) { return value ? new Date(value).toLocaleString() : '—' }
function categoryName(category: FavoriteCategory) { return category.name || `Category ${category.category}` }

function stateLabel(category: FavoriteCategory) {
  if (category.state === 'waiting_cooldown') return 'Waiting for cooldown'
  if (category.state === 'running') return category.full ? 'Running full re-sync' : 'Running sync'
  if (category.state === 'queued') return category.queued_full ? 'Full re-sync queued' : 'Sync queued'
  if (category.last_outcome === 'failed') return 'Failed'
  if (category.last_outcome === 'success') return 'Completed'
  return 'Idle'
}

export default function Collector() {
  const collector = useCollector()
  const [category, setCategory] = useState('all')
  const [full, setFull] = useState(false)
  const { connection, snapshot } = collector
  const status = snapshot?.status
  const connected = !!connection?.status && !collector.connectionUnknown
  const connectionLabel = collector.connectionUnknown ? 'Status unavailable'
    : !connection ? 'Checking connection…'
    : !connection.configured ? 'Not configured'
    : !connection.reachable ? 'Unreachable'
    : connection.authenticated === false ? 'Access rejected'
    : connected ? 'Connected' : 'Status unavailable'
  const favoriteErrors = status?.favorites.categories.filter((item) => item.last_error) ?? []

  return (
    <section className="collector-page" aria-labelledby="collector-title">
      <div className="collector-toolbar">
        <div className="page-heading">
          <p className="eyebrow">Collection health</p>
          <h1 id="collector-title">Collector</h1>
          <p>Sync Panda favorites, manage downloads, and follow metadata collection.</p>
        </div>
        <button className="button" type="button" disabled={collector.checking} onClick={collector.refresh}>{collector.checking ? 'Refreshing…' : 'Refresh'}</button>
      </div>

      <div className="collector-connection">
        <span className={`collector-badge${connected ? ' collector-badge-ok' : ''}`}>{connectionLabel}</span>
        {connection?.configured && <span>Reachable: {collector.connectionUnknown ? 'Unknown' : connection.reachable ? 'Yes' : 'No'} · API access: {collector.connectionUnknown || connection.authenticated === null ? 'Unknown' : connection.authenticated ? 'Accepted' : 'Rejected'}</span>}
        <span>Refreshes every 5 seconds</span>
      </div>

      {collector.error && <div className="error-banner" role="alert"><p>{collector.error}</p></div>}
      {connection && !connection.configured && <div className="collector-panel"><h2>Connect a collector</h2><p>Configure Tana’s collector URL and API token, then restart Tana to enable favorite sync, downloads, and statistics.</p></div>}
      {collector.stale && <p className="collector-stale" role="status">Statistics are stale. Last updated {date(snapshot?.at)}.</p>}

      {status && <>
        <Downloads available={connected} refreshKey={collector.revision} />
        <div className="collector-panel">
          <div className="collector-section-heading"><h2>Collector inventory</h2><span>All accounts and collection paths</span></div>
          <dl className="collector-metrics">
            <div><dt>Gallery references</dt><dd>{status.inventory.gallery_references.toLocaleString()}</dd></div>
            <div><dt>Metadata available</dt><dd>{status.inventory.metadata_available.toLocaleString()}</dd></div>
            <div><dt>Metadata pending</dt><dd>{status.inventory.metadata_pending.toLocaleString()}</dd></div>
            <div><dt>Metadata failed</dt><dd>{status.inventory.metadata_failed.toLocaleString()}</dd></div>
          </dl>
          <p className="field-help">Pending and failed counts cover references without retained metadata. Explicit fetch requests: {status.inventory.fetches_pending.toLocaleString()} pending · {status.inventory.fetches_failed.toLocaleString()} failed.</p>
        </div>

        <div className="collector-panel">
          <div className="collector-section-heading"><h2>Panda favorites</h2><span>{status.favorites.categories.reduce((sum, item) => sum + item.favorites, 0).toLocaleString()} collected favorites</span></div>
          <p className="collector-account">{status.favorites.host} · Account {status.favorites.account_key}</p>
          <form className="collector-sync" onSubmit={(event) => { event.preventDefault(); if (!collector.disabled) void collector.sync(category, full) }}>
            <label className="form-field">Favorite category
              <select className="text-input" value={category} onChange={(event) => setCategory(event.target.value)} disabled={collector.disabled}>
                <option value="all">All ten categories</option>
                {status.favorites.categories.map((item) => <option key={item.category} value={item.category}>{item.name ? `${item.category} · ${item.name}` : categoryName(item)}</option>)}
              </select>
            </label>
            <label className="form-field">Sync mode
              <select className="text-input" value={full ? 'full' : 'incremental'} onChange={(event) => setFull(event.target.value === 'full')} disabled={collector.disabled}>
                <option value="incremental">Incremental sync</option>
                <option value="full">Full re-sync</option>
              </select>
            </label>
            <button className="button button-primary" type="submit" disabled={collector.disabled}>{collector.pending ? 'Requesting…' : full ? 'Full re-sync favorites' : 'Sync favorites'}</button>
          </form>
          <p className="field-help">{status.favorites.downloads && status.favorites.downloads.baseline_state !== 'ready'
            ? 'Until the baseline is complete, sync collects all ten categories without downloading favorites.'
            : full ? 'Full re-sync reconciles removed favorites. Collected gallery references and metadata are retained.' : 'Collect newest favorites.'}</p>
          {collector.notice && <p className="collector-notice" role="status">{collector.notice}</p>}
          {collector.requestError && <p className="error-message" role="alert">{collector.requestError}</p>}
          {status.favorites.downloads && <FavoriteDownloads status={status.favorites.downloads} categories={status.favorites.categories} available={connected} onSaved={collector.downloadSettingsSaved} />}
          <div className="collector-table-scroll">
            <table className="collector-table">
              <caption className="collector-table-caption">Favorite categories for the current account</caption>
              <thead><tr><th scope="col">Category</th><th scope="col">Favorites</th><th scope="col">Sync state</th><th scope="col">Sync progress</th><th scope="col">Last successful sync</th></tr></thead>
              <tbody>{status.favorites.categories.map((item) => (
                <tr key={item.category}>
                  <th scope="row">{categoryName(item)}{item.name && <span className="collector-secondary">Category {item.category}</span>}</th>
                  <td>{item.last_synced_at || item.last_saved_at ? item.favorites.toLocaleString() : '—'}</td>
                  <td><span className="collector-state">{stateLabel(item)}</span>
                    {item.retry_at && <span className="collector-secondary">Retry after {date(item.retry_at)}</span>}
                    {item.queued && item.state !== 'queued' && <span className="collector-secondary">{item.queued_full ? 'Full re-sync' : 'Sync'} also queued</span>}
                    {item.state === 'idle' && item.finished_at && <span className="collector-secondary">{date(item.finished_at)}</span>}
                  </td>
                  <td>{item.started_at ? <>
                    <span>{(item.entries_saved ?? 0).toLocaleString()} entries saved · {(item.pages_saved ?? 0).toLocaleString()} pages</span>
                    <span className="collector-secondary">{item.last_saved_at ? `Last save ${date(item.last_saved_at)}` : 'Awaiting first page'}</span>
                  </> : '—'}</td>
                  <td>{item.last_synced_at ? date(item.last_synced_at) : 'Never synced'}</td>
                </tr>
              ))}</tbody>
            </table>
          </div>
        </div>

        <div className="collector-panel">
          <h2>Diagnostics</h2>
          <p>{status.upstream_cooldown_until ? `Panda requests paused until ${date(status.upstream_cooldown_until)}.` : 'No active Panda cooldown.'}</p>
          {status.metadata_retry_at && <p>Metadata retry after {date(status.metadata_retry_at)}.</p>}
          {status.metadata_last_error && <p className="error-message">Metadata collection: {status.metadata_last_error}</p>}
          {favoriteErrors.length > 0 && <><h3>Latest favorite sync errors</h3><ul className="collector-errors">{favoriteErrors.map((item) => <li key={item.category}><strong>{categoryName(item)}</strong><span>{item.last_error}</span><small>{date(item.last_error_at)}</small></li>)}</ul></>}
          {status.metadata_errors.length > 0 && <><h3>Recent metadata errors</h3><ul className="collector-errors">{status.metadata_errors.map((item) => <li key={item.gallery_id}><strong>Gallery {item.gallery_id}</strong><span>{item.error}</span><small>{date(item.at)}</small></li>)}</ul></>}
          {!status.metadata_last_error && favoriteErrors.length === 0 && status.metadata_errors.length === 0 && <p className="field-help">No recent collection errors.</p>}
        </div>
        {!collector.stale && <p className="field-help">Last updated {date(snapshot?.at)}.</p>}
      </>}
    </section>
  )
}
