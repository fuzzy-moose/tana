import { useEffect, useRef, useState } from 'react'
import { previewRefresh, removeMissingSources } from './refreshApi'
import type { RefreshPlan, RefreshResult } from './refreshApi'
import './SourceCleanup.css'

export default function LibraryRefresh({ libraryID }: { libraryID?: number }) {
  const [plan, setPlan] = useState<RefreshPlan | null>(null)
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [attempted, setAttempted] = useState(false)
  const [result, setResult] = useState<RefreshResult | null>(null)
  const [error, setError] = useState('')
  const [revision, setRevision] = useState(0)
  const submitting = useRef(false)

  useEffect(() => {
    const controller = new AbortController()
    previewRefresh(libraryID, controller.signal).then((next) => {
      if (controller.signal.aborted) return
      setPlan(next)
      setSelected(new Set(next.candidates.map((candidate) => candidate.source_id)))
    }).catch((error: Error) => {
      if (!controller.signal.aborted) setError(error.message)
    }).finally(() => {
      if (!controller.signal.aborted) setLoading(false)
    })
    return () => controller.abort()
  }, [libraryID, revision])

  function checkAgain() {
    setLoading(true)
    setPlan(null)
    setSelected(new Set())
    setAttempted(false)
    setResult(null)
    setError('')
    setRevision((value) => value + 1)
  }

  function toggle(id: number) {
    setSelected((current) => {
      const next = new Set(current)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  async function removeSelected() {
    if (!plan || attempted || submitting.current || selected.size === 0) return
    submitting.current = true
    setBusy(true)
    setAttempted(true)
    setError('')
    try {
      setResult(await removeMissingSources(plan.plan_id, [...selected]))
    } catch (error) {
      setError(error instanceof Error ? error.message : 'Could not complete library refresh.')
    } finally {
      setBusy(false)
      submitting.current = false
    }
  }

  const locked = loading || busy || attempted
  const removed = new Set(result?.removed)
  const failures = new Map(result?.failed.map((item) => [item.source_id, item.reason]))

  return <section className="source-cleanup" aria-labelledby="refresh-title">
    <a className="cleanup-back" href="#/libraries">← Libraries</a>
    <div className="cleanup-toolbar">
      <div className="page-heading">
        <p className="eyebrow">Missing sources</p>
        <h1 id="refresh-title">{libraryID === undefined ? 'Refresh all libraries' : 'Refresh library'}</h1>
        <p>Remove catalog entries for archives and directories that no longer exist.</p>
      </div>
      <button className="button" type="button" disabled={loading || busy} onClick={checkAgain}>Check again</button>
    </div>
    <p className="cleanup-help">Unavailable storage is preserved. Removal is blocked for a library when no cataloged source can be confirmed present, even if all sources were intentionally deleted.</p>
    {loading && <p role="status">Checking source paths…</p>}
    {error && <p className="error-message" role="alert">{error}</p>}
    {busy && <p role="status">Rechecking storage and removing missing sources…</p>}
    {result && <p className="cleanup-notice" role="status">
      {result.removed.length} {result.removed.length === 1 ? 'source' : 'sources'} removed. {result.failed.length} could not be removed.
    </p>}
    {attempted && !busy && <p className="cleanup-help">Check again for a fresh preview before another removal.</p>}

    {plan && <>
      {plan.skipped.length > 0 && <div className="cleanup-confirmation" role="note" aria-label="Preserved entries">
        <h2>Preserved entries</h2>
        <ul>{plan.skipped.map((item) => <li key={`${item.library_id}:${item.source_id ?? 'library'}`}>
          <p><strong>{item.library_name}</strong> — {item.reason}</p>
          <p className="cleanup-path">{item.path}</p>
        </li>)}</ul>
      </div>}
      {plan.candidates.length === 0 && <p className="cleanup-empty" role="status">No missing sources eligible for removal.</p>}
      {plan.candidates.length > 0 && <>
        {!attempted && <div className="cleanup-selection">
          <p><strong>{selected.size} {selected.size === 1 ? 'source' : 'sources'} selected</strong></p>
          <div className="button-group">
            <button className="button" type="button" disabled={locked} onClick={() => setSelected(new Set(plan.candidates.map((candidate) => candidate.source_id)))}>Select all</button>
            <button className="button" type="button" disabled={locked} onClick={() => setSelected(new Set())}>Clear selection</button>
          </div>
        </div>}
        <ul className="cleanup-candidates" aria-label="Missing sources">
          {plan.candidates.map((candidate) => <li className="cleanup-candidate" key={candidate.source_id}>
            <div className="cleanup-candidate-heading">
              <label><input type="checkbox" checked={selected.has(candidate.source_id)} disabled={locked} onChange={() => toggle(candidate.source_id)} aria-label={`Select ${candidate.path}`} />{candidate.library_name}</label>
              {attempted && !busy && <p className={failures.has(candidate.source_id) ? 'cleanup-failure' : 'cleanup-outcome'}>
                {removed.has(candidate.source_id) ? 'Removed from catalog' : failures.has(candidate.source_id) ? `Kept: ${failures.get(candidate.source_id)}` : selected.has(candidate.source_id) ? 'Outcome unknown; check again.' : 'Excluded from removal'}
              </p>}
            </div>
            <p className="cleanup-path">{candidate.path}</p>
            {candidate.galleries.length > 0 ? <ul className="cleanup-help" aria-label={`Affected galleries for ${candidate.path}`}>
              {candidate.galleries.map((gallery) => <li key={gallery.id}>
                {gallery.title} — {gallery.deleted ? 'gallery will be deleted' : `${gallery.pages_removed} ${gallery.pages_removed === 1 ? 'page' : 'pages'} will be removed; gallery will be kept`}
              </li>)}
            </ul> : <p className="cleanup-help">No affected galleries.</p>}
          </li>)}
        </ul>
        {!attempted && <div className="cleanup-confirmation" role="group" aria-label="Confirm source removal">
          <h2>Remove selected sources?</h2>
          <p>Their source-linked galleries, metadata, and reading progress will be lost. Independent galleries lose affected pages and remain even when empty. A renamed or moved source will be imported as new by Scan.</p>
          <div className="button-group">
            <a className="button" href="#/libraries">Cancel</a>
            <button className="button cleanup-delete" type="button" disabled={locked || selected.size === 0} onClick={() => void removeSelected()}>Remove {selected.size} {selected.size === 1 ? 'source' : 'sources'}</button>
          </div>
        </div>}
      </>}
    </>}
  </section>
}
