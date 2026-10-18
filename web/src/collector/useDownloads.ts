import { useEffect, useState } from 'react'
import { changeDownload, downloadState, listDownloads, parseGalleryURL, submitDownload } from './downloads'
import { useMutation } from './useMutation'
import type { DownloadCounts, DownloadFilter, DownloadJob, DownloadStorage } from './downloads'

export function useDownloads(available: boolean, refreshKey: number, interval: number, page: number, filter: DownloadFilter) {
  const [snapshot, setSnapshot] = useState<{ jobs: DownloadJob[], counts: DownloadCounts, storage?: DownloadStorage, page: number, filter: DownloadFilter } | null>(null)
  const [checking, setChecking] = useState(true)
  const [error, setError] = useState('')
  const [revision, setRevision] = useState(0)
  const mutation = useMutation()
  const { pending } = mutation

  useEffect(() => {
    if (!available || pending) return
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function poll() {
      setChecking(true)
      try {
        const result = await listDownloads(page, controller.signal, filter)
        if (controller.signal.aborted) return
        if (!Array.isArray(result.jobs)) throw new Error('The collector returned an unexpected download list.')
        setSnapshot({ jobs: result.jobs, counts: result.counts, storage: result.storage, page, filter })
        setError('')
      } catch (error) {
        if (!controller.signal.aborted) setError((error as Error).message)
      } finally {
        if (!controller.signal.aborted) {
          setChecking(false)
          timer = setTimeout(poll, interval)
        }
      }
    }
    void poll()
    return () => { controller.abort(); clearTimeout(timer) }
  }, [available, pending, page, filter, revision, refreshKey, interval])

  function perform(operation: (signal: AbortSignal) => Promise<string>) {
    if (!available) return Promise.resolve(false)
    return mutation.run(operation, { onSuccess: (message) => message, onSettled: () => setRevision((value) => value + 1) })
  }

  // The previous response's counts stay until the next arrives, so the tabs and pagination keep their totals.
  const current = snapshot?.page === page && snapshot.filter === filter ? snapshot : null
  return {
    jobs: current?.jobs ?? [],
    counts: snapshot?.counts,
    storage: snapshot?.storage,
    loaded: !!current, checking, pending, error, requestError: mutation.error, notice: mutation.notice,
    stale: !!snapshot && (!available || !!error),
    disabled: !available || pending,
    submit: (value: string) => perform(async (signal) => {
      const job = await submitDownload(parseGalleryURL(value), signal)
      return `Gallery ${job.gallery_id}: ${downloadState(job)}.`
    }),
    change: (id: number, action: 'cancel' | 'retry' | 'delete') => perform(async (signal) => {
      const job = await changeDownload(id, action, signal)
      return job ? `Gallery ${id}: ${downloadState(job)}.` : `Gallery ${id}: download deleted.`
    }),
  }
}
