import { useEffect, useRef, useState } from 'react'
import { changeDownload, downloadPageSize, downloadState, listDownloads, parseGalleryURL, submitDownload } from './downloads'
import type { DownloadCounts, DownloadFilter, DownloadJob } from './downloads'

export function useDownloads(available: boolean, refreshKey: number) {
  const [snapshot, setSnapshot] = useState<{ jobs: DownloadJob[], counts: DownloadCounts, offset: number, filter: DownloadFilter } | null>(null)
  const [offset, setOffset] = useState(0)
  const [filter, setFilter] = useState<DownloadFilter>('')
  const [checking, setChecking] = useState(true)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const [requestError, setRequestError] = useState('')
  const [notice, setNotice] = useState('')
  const [revision, setRevision] = useState(0)
  const mutation = useRef<AbortController | null>(null)

  useEffect(() => () => mutation.current?.abort(), [])
  useEffect(() => {
    if (!available || pending) return
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function poll() {
      setChecking(true)
      try {
        const result = await listDownloads(offset, controller.signal, filter)
        if (controller.signal.aborted) return
        if (!Array.isArray(result.jobs)) throw new Error('The collector returned an unexpected download list.')
        if (result.jobs.length === 0 && offset > 0) {
          setOffset((value) => Math.max(0, value - downloadPageSize))
          return
        }
        setSnapshot({ jobs: result.jobs, counts: result.counts, offset, filter })
        setError('')
      } catch (error) {
        if (!controller.signal.aborted) setError((error as Error).message)
      } finally {
        if (!controller.signal.aborted) {
          setChecking(false)
          timer = setTimeout(poll, 5000)
        }
      }
    }
    void poll()
    return () => { controller.abort(); clearTimeout(timer) }
  }, [available, pending, offset, filter, revision, refreshKey])

  async function perform(operation: (signal: AbortSignal) => Promise<string>) {
    if (mutation.current || !available) return false
    const controller = new AbortController()
    mutation.current = controller
    setPending(true)
    setRequestError('')
    setNotice('')
    try {
      const message = await operation(controller.signal)
      if (controller.signal.aborted) return false
      setNotice(message)
      return true
    } catch (error) {
      if (!controller.signal.aborted) setRequestError((error as Error).message)
      return false
    } finally {
      mutation.current = null
      if (!controller.signal.aborted) {
        setPending(false)
        setRevision((value) => value + 1)
      }
    }
  }

  const current = snapshot?.offset === offset && snapshot.filter === filter ? snapshot : null
  return {
    jobs: current?.jobs.slice(0, downloadPageSize) ?? [],
    counts: snapshot?.counts,
    filter,
    selectFilter: (value: DownloadFilter) => { setFilter(value); setOffset(0); setError('') },
    hasNext: (current?.jobs.length ?? 0) > downloadPageSize,
    loaded: !!current, offset, checking, pending, error, requestError, notice,
    stale: !!snapshot && (!available || !!error),
    disabled: !available || pending,
    refresh: () => setRevision((value) => value + 1),
    previous: () => setOffset((value) => Math.max(0, value - downloadPageSize)),
    next: () => setOffset((value) => value + downloadPageSize),
    submit: (value: string) => perform(async (signal) => {
      const job = await submitDownload(parseGalleryURL(value), signal)
      if (!signal.aborted) { setOffset(0); setFilter('') }
      return `Gallery ${job.gallery_id}: ${downloadState(job)}.`
    }),
    change: (id: number, action: 'cancel' | 'retry' | 'delete') => perform(async (signal) => {
      const job = await changeDownload(id, action, signal)
      return job ? `Gallery ${id}: ${downloadState(job)}.` : `Gallery ${id}: download deleted.`
    }),
  }
}
