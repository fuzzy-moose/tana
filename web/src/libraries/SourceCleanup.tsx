import { useEffect, useRef, useState } from 'react'
import { deleteSupersededSources, previewCleanup } from './cleanupApi'
import type { CleanupPlan, CleanupResult, CleanupSource } from './cleanupApi'
import './SourceCleanup.css'

function size(bytes: number) {
  if (bytes < 1024) return `${bytes.toLocaleString()} B`
  if (bytes < 1024 ** 2) return `${(bytes / 1024).toFixed(1)} KiB`
  if (bytes < 1024 ** 3) return `${(bytes / 1024 ** 2).toFixed(1)} MiB`
  return `${(bytes / 1024 ** 3).toFixed(1)} GiB`
}

function SourceDetails({ source, label }: { source: CleanupSource, label: string }) {
  return <div className="cleanup-source">
    <p className="cleanup-label">{label}</p>
    <h3>{source.title || `Panda gallery ${source.panda_id}`}</h3>
    <p className="cleanup-metadata">{source.library_name} · Panda {source.panda_id}</p>
    <p className="cleanup-path">{source.path}</p>
  </div>
}

export default function SourceCleanup() {
  const [plan, setPlan] = useState<CleanupPlan | null>(null)
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const [attempted, setAttempted] = useState(false)
  const [result, setResult] = useState<CleanupResult | null>(null)
  const [error, setError] = useState('')
  const [refresh, setRefresh] = useState(0)
  const submitting = useRef(false)

  useEffect(() => {
    const controller = new AbortController()
    previewCleanup(controller.signal).then((next) => {
      if (controller.signal.aborted) return
      setPlan(next)
      setSelected(new Set(next.candidates.map((candidate) => candidate.source.id)))
    }).catch((error: Error) => {
      if (!controller.signal.aborted) setError(error.message)
    }).finally(() => {
      if (!controller.signal.aborted) setLoading(false)
    })
    return () => controller.abort()
  }, [refresh])

  function refreshPreview() {
    setLoading(true)
    setPlan(null)
    setSelected(new Set())
    setConfirming(false)
    setAttempted(false)
    setResult(null)
    setError('')
    setRefresh((value) => value + 1)
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
    if (!plan || !confirming || attempted || submitting.current || selected.size === 0) return
    submitting.current = true
    setBusy(true)
    setAttempted(true)
    setConfirming(false)
    setError('')
    try {
      setResult(await deleteSupersededSources(plan.plan_id, [...selected]))
    } catch (error) {
      setError(error instanceof Error ? error.message : 'Could not complete cleanup.')
    } finally {
      setBusy(false)
      submitting.current = false
    }
  }

  const selectedCandidates = plan?.candidates.filter((candidate) => selected.has(candidate.source.id)) ?? []
  const totalSize = selectedCandidates.reduce((total, candidate) => total + candidate.size_bytes, 0)
  const locked = loading || busy || attempted || confirming
  const visibleCandidates = confirming ? selectedCandidates : plan?.candidates ?? []
  const failures = new Map(result?.failed.map((item) => [item.source_id, item.reason]))
  const deleted = new Set(result?.deleted)

  return <section className="source-cleanup" aria-labelledby="cleanup-title">
    <a className="cleanup-back" href="#/libraries">← Libraries</a>
    <div className="cleanup-toolbar">
      <div className="page-heading">
        <p className="eyebrow">All libraries</p>
        <h1 id="cleanup-title">Clean up older versions</h1>
        <p>Delete older archives when a newer version is present in any registered library.</p>
      </div>
      <button className="button" type="button" disabled={loading || busy || confirming} onClick={refreshPreview}>Refresh preview</button>
    </div>
    <p className="cleanup-help">Matches use Panda IDs in filenames and collected parent references. Review each older archive and the newer source that will remain.</p>
    {loading && <p role="status">Checking sources across all libraries…</p>}
    {error && <p className="error-message" role="alert">{error}</p>}
    {busy && <p role="status">Permanently deleting selected archives…</p>}
    {result && <p className="cleanup-notice" role="status">
      {result.deleted.length} {result.deleted.length === 1 ? 'archive' : 'archives'} deleted. {result.failed.length} could not be deleted.
    </p>}
    {attempted && !busy && <p className="cleanup-help">Refresh the preview to check current sources before another cleanup.</p>}

    {plan && <>
      {plan.candidates.length === 0 && <p className="cleanup-empty" role="status">No eligible older archives found.</p>}
      {plan.candidates.length > 0 && <>
        {!attempted && <div className="cleanup-selection">
          <p><strong>{selected.size} {selected.size === 1 ? 'archive' : 'archives'} selected · {size(totalSize)}</strong></p>
          {!confirming && <div className="button-group">
            <button className="button" type="button" disabled={locked} onClick={() => setSelected(new Set(plan.candidates.map((candidate) => candidate.source.id)))}>Select all</button>
            <button className="button" type="button" disabled={locked} onClick={() => setSelected(new Set())}>Clear selection</button>
            <button className="button cleanup-delete" type="button" disabled={locked || selected.size === 0} onClick={() => setConfirming(true)}>Delete selected…</button>
          </div>}
        </div>}
        {confirming && <div className="cleanup-confirmation" role="group" aria-label="Confirm permanent deletion">
          <h2>Permanently delete {selected.size} {selected.size === 1 ? 'archive' : 'archives'}?</h2>
          <p>The archives listed below ({size(totalSize)}), their catalog records, and source-linked galleries will be deleted. Files will not go to trash. This cannot be undone.</p>
          <div className="button-group">
            <button autoFocus className="button" type="button" onClick={() => setConfirming(false)}>Cancel</button>
            <button className="button cleanup-delete" type="button" onClick={() => void removeSelected()}>Permanently delete {selected.size} {selected.size === 1 ? 'archive' : 'archives'}</button>
          </div>
        </div>}
        <ul className="cleanup-candidates" aria-label={confirming ? 'Archives to permanently delete' : 'Older archives'}>
          {visibleCandidates.map(({ source, replacement, size_bytes }) => {
            const failure = failures.get(source.id)
            return <li className="cleanup-candidate" key={source.id}>
              <div className="cleanup-candidate-heading">
                <label><input type="checkbox" checked={selected.has(source.id)} disabled={locked} onChange={() => toggle(source.id)} aria-label={`Select ${source.path}`} />{size(size_bytes)}</label>
                {result && <p className={failure ? 'cleanup-failure' : 'cleanup-outcome'}>{deleted.has(source.id) ? 'Permanently deleted' : failure ? `Could not delete: ${failure}` : selected.has(source.id) ? 'Outcome unknown; refresh preview.' : 'Kept — excluded from cleanup'}</p>}
              </div>
              <div className="cleanup-pair">
                <SourceDetails source={source} label="Older archive" />
                <SourceDetails source={replacement} label="Newer source to keep" />
              </div>
            </li>
          })}
        </ul>
      </>}
      {plan.skipped.length > 0 && <details className="cleanup-skipped">
        <summary>{plan.skipped.length} {plan.skipped.length === 1 ? 'source' : 'sources'} skipped</summary>
        <ul>{plan.skipped.map((item) => <li key={item.source_id}>
          <p className="cleanup-path">{item.path}</p>
          <p>{item.reason}</p>
        </li>)}</ul>
      </details>}
    </>}
  </section>
}
