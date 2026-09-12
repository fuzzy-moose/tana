import { useEffect, useRef, useState } from 'react'
import { getFavoritesStatus, syncFavorites } from './api'
import type { FavoriteCategory } from './api'
import { useCollectorResource } from './useCollectorResource'
import FavoriteDownloads from './FavoriteDownloads'
import ResourceStatus from './ResourceStatus'

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

export default function Favorites({ available, refreshKey }: { available: boolean, refreshKey: number }) {
  const favorites = useCollectorResource(getFavoritesStatus, available, refreshKey)
  const [category, setCategory] = useState('all')
  const [full, setFull] = useState(false)
  const [pending, setPending] = useState(false)
  const [notice, setNotice] = useState('')
  const [requestError, setRequestError] = useState('')
  const mutation = useRef<AbortController | null>(null)
  useEffect(() => () => mutation.current?.abort(), [])
  const status = favorites.data
  const disabled = !available || favorites.stale || pending || !status

  async function sync(category: string, full: boolean) {
    if (mutation.current || disabled) return
    const baseline = status?.downloads?.baseline_state
    const controller = new AbortController()
    mutation.current = controller
    setPending(true)
    setNotice('')
    setRequestError('')
    try {
      await syncFavorites(category, full, controller.signal)
      if (!controller.signal.aborted) setNotice(baseline && baseline !== 'ready'
        ? 'Baseline sync requested for all ten categories.'
        : `${full ? 'Full re-sync' : 'Sync'} requested for ${category === 'all' ? 'all categories' : `category ${category}`}.`)
    } catch (error) {
      if (!controller.signal.aborted) setRequestError((error as Error).message)
    } finally {
      mutation.current = null
      if (!controller.signal.aborted) { setPending(false); favorites.refresh() }
    }
  }

  function downloadSettingsSaved(categories: number[]) {
    if (status?.downloads) favorites.update({ ...status, downloads: { ...status.downloads, categories } })
  }

  return <>
    <ResourceStatus name="Favorite statistics" loaded={!!status} available={available} {...favorites} />
    {status && (
      <div className="collector-panel">
        <div className="collector-section-heading"><h2>Panda favorites</h2><span>{status.categories.reduce((sum, item) => sum + item.favorites, 0).toLocaleString()} collected favorites</span></div>
        <p className="collector-account">{status.host} · Account {status.account_key}</p>
        <form className="collector-sync" onSubmit={(event) => { event.preventDefault(); if (!disabled) void sync(category, full) }}>
          <label className="form-field">Favorite category
            <select className="text-input" value={category} onChange={(event) => setCategory(event.target.value)} disabled={disabled}>
              <option value="all">All ten categories</option>
              {status.categories.map((item) => <option key={item.category} value={item.category}>{item.name ? `${item.category} · ${item.name}` : categoryName(item)}</option>)}
            </select>
          </label>
          <label className="form-field">Sync mode
            <select className="text-input" value={full ? 'full' : 'incremental'} onChange={(event) => setFull(event.target.value === 'full')} disabled={disabled}>
              <option value="incremental">Incremental sync</option>
              <option value="full">Full re-sync</option>
            </select>
          </label>
          <button className="button button-primary" type="submit" disabled={disabled}>{pending ? 'Requesting…' : full ? 'Full re-sync favorites' : 'Sync favorites'}</button>
        </form>
        <p className="field-help">{status.downloads && status.downloads.baseline_state !== 'ready'
          ? 'Until the baseline is complete, sync collects all ten categories without downloading favorites.'
          : full ? 'Full re-sync reconciles removed favorites. Collected gallery references and metadata are retained.' : 'Collect newest favorites.'}</p>
        {notice && <p className="collector-notice" role="status">{notice}</p>}
        {requestError && <p className="error-message" role="alert">{requestError}</p>}
        {status.downloads && <FavoriteDownloads status={status.downloads} categories={status.categories} available={!disabled} onSaved={downloadSettingsSaved} />}
        <div className="collector-table-scroll">
          <table className="collector-table">
            <caption className="collector-table-caption">Favorite categories for the current account</caption>
            <thead><tr><th scope="col">Category</th><th scope="col">Favorites</th><th scope="col">Sync state</th><th scope="col">Sync progress</th><th scope="col">Last successful sync</th></tr></thead>
            <tbody>{status.categories.map((item) => (
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

        {status.categories.some((item) => item.last_error) && <>
          <h3>Latest favorite sync errors</h3>
          <ul className="collector-errors">{status.categories.filter((item) => item.last_error).map((item) => <li key={item.category}><strong>{categoryName(item)}</strong><span>{item.last_error}</span><small>{date(item.last_error_at)}</small></li>)}</ul>
        </>}
      </div>
    )}
  </>
}
