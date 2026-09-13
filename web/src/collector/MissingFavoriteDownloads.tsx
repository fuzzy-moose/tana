import { useEffect, useRef, useState } from 'react'
import { APIError } from '../api'
import type { FavoriteCategory } from './api'
import { previewMissingFavoriteDownloads, submitMissingFavoriteDownloads } from './missingFavoriteDownloads'
import type { MissingFavoriteDownloadCounts, MissingFavoriteDownloadPreview } from './missingFavoriteDownloads'

export default function MissingFavoriteDownloads({ category, available, onClose, onSubmittingChange }: {
  category: FavoriteCategory
  available: boolean
  onClose: () => void
  onSubmittingChange: (submitting: boolean) => void
}) {
  const [plan, setPlan] = useState<MissingFavoriteDownloadPreview | null>(null)
  const [result, setResult] = useState<MissingFavoriteDownloadCounts | null>(null)
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [attempted, setAttempted] = useState(false)
  const [error, setError] = useState('')
  const [revision, setRevision] = useState(0)
  const mutation = useRef<AbortController | null>(null)
  const heading = useRef<HTMLHeadingElement | null>(null)

  useEffect(() => {
    heading.current?.focus()
    return () => mutation.current?.abort()
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    previewMissingFavoriteDownloads(category.category, controller.signal).then((preview) => {
      if (!controller.signal.aborted) setPlan(preview)
    }).catch((error: Error) => {
      if (!controller.signal.aborted) setError(error.message)
    }).finally(() => {
      if (!controller.signal.aborted) setLoading(false)
    })
    return () => controller.abort()
  }, [category.category, revision])

  function refreshPreview() {
    if (!available || loading || mutation.current) return
    setPlan(null)
    setResult(null)
    setError('')
    setAttempted(false)
    setLoading(true)
    setRevision((value) => value + 1)
  }

  async function confirm() {
    if (!plan || !available || mutation.current || result) return
    const controller = new AbortController()
    mutation.current = controller
    setSubmitting(true)
    onSubmittingChange(true)
    setAttempted(true)
    setError('')
    try {
      const accepted = await submitMissingFavoriteDownloads(category.category, plan.plan_id, controller.signal)
      if (!controller.signal.aborted) setResult(accepted)
    } catch (error) {
      if (!controller.signal.aborted) {
        setError((error as Error).message)
        if (error instanceof APIError && error.code === 'missing_download_preview_invalid') setPlan(null)
      }
    } finally {
      mutation.current = null
      if (!controller.signal.aborted) {
        setSubmitting(false)
        onSubmittingChange(false)
      }
    }
  }

  const counts = result ?? plan
  const name = category.name || `Category ${category.category}`
  const blocked = !available || loading || submitting

  return <section className="missing-favorite-downloads" aria-labelledby="missing-favorite-title">
    <div className="collector-section-heading">
      <h3 id="missing-favorite-title" ref={heading} tabIndex={-1}>Download missing favorites · {name}</h3>
      <button className="button" type="button" disabled={submitting} onClick={onClose}>{result ? 'Close' : 'Cancel'}</button>
    </div>
    <p>Uses all collected favorites in this category. Matches exact Panda IDs in source names across all registered Tana libraries, including offline libraries.</p>
    <p className="field-help">Archives stay in the collector. Retained archives do not count as sources in a Tana library.</p>
    {loading && <p role="status">Checking missing favorites…</p>}
    {error && <p className="error-message" role="alert">{error}</p>}
    {submitting && <p role="status">Submitting reviewed downloads…</p>}
    {result && <p className="collector-notice" role="status">Download requests accepted. The collector will continue after you leave this page.</p>}
    {counts && <>
      <p>{counts.total.toLocaleString()} collected favorites · {counts.present.toLocaleString()} present in Tana · {counts.missing.toLocaleString()} missing</p>
      <dl className="collector-metrics missing-favorite-counts">
        <div><dt>{result ? 'New downloads queued' : 'New downloads'}</dt><dd>{counts.new_downloads.toLocaleString()}</dd></div>
        <div><dt>{result ? 'Queued or running jobs reused' : 'Queued or running jobs to reuse'}</dt><dd>{counts.existing_jobs.toLocaleString()}</dd></div>
        <div><dt>{result ? 'Retained archives reused' : 'Retained archives to reuse'}</dt><dd>{counts.retained_archives.toLocaleString()}</dd></div>
        <div><dt>Failed jobs skipped</dt><dd>{counts.failed.toLocaleString()}</dd></div>
        <div><dt>Cancelled jobs skipped</dt><dd>{counts.cancelled.toLocaleString()}</dd></div>
        <div><dt>Deleting jobs skipped</dt><dd>{counts.deleting.toLocaleString()}</dd></div>
      </dl>
      {(counts.failed > 0 || counts.cancelled > 0) && <p className="field-help">Retry failed or cancelled jobs explicitly from Downloads.</p>}
      {!result && <p className="field-help">Confirmation keeps this reviewed set and skips favorites now present in Tana. Jobs created meanwhile are reused.</p>}
      {!result && counts.missing === 0 && <p role="status">No missing favorites in this category.</p>}
    </>}
    {attempted && error && plan && <p className="field-help">You can safely retry this confirmation; existing requests will be reused.</p>}
    <div className="button-group">
      {!result && plan && plan.missing > 0 && <button className="button button-primary" type="button" disabled={blocked} onClick={() => void confirm()}>
        {submitting ? 'Submitting…' : attempted ? 'Retry confirmation' : 'Confirm downloads'}
      </button>}
      {!result && <button className="button" type="button" disabled={blocked} onClick={refreshPreview}>Refresh preview</button>}
      {(result || error || counts && counts.missing > 0) && <a className="button" href="#/collector/downloads">View Downloads</a>}
    </div>
  </section>
}
