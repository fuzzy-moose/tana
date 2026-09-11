import { useEffect, useRef, useState } from 'react'
import { collectorMessages, getCollectorStatus, syncFavorites } from './api'
import type { CollectorStatus, ConnectionStatus, SitemapStatus } from './api'

export function useCollector() {
  const [connection, setConnection] = useState<ConnectionStatus | null>(null)
  const [snapshot, setSnapshot] = useState<{ status: CollectorStatus, at: string } | null>(null)
  const [statusError, setStatusError] = useState('')
  const [requestError, setRequestError] = useState('')
  const [notice, setNotice] = useState('')
  const [checking, setChecking] = useState(true)
  const [pending, setPending] = useState(false)
  const [revision, setRevision] = useState(0)
  const mutation = useRef<AbortController | null>(null)

  useEffect(() => () => mutation.current?.abort(), [])
  useEffect(() => {
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function poll() {
      setChecking(true)
      try {
        const next = await getCollectorStatus(controller.signal)
        if (controller.signal.aborted) return
        setConnection(next)
        setStatusError('')
        if (next.status) setSnapshot({ status: next.status, at: next.checked_at })
        if (!next.configured) setSnapshot(null)
      } catch (error) {
        if (!controller.signal.aborted) setStatusError((error as Error).message)
      } finally {
        if (!controller.signal.aborted) {
          setChecking(false)
          timer = setTimeout(poll, 5000)
        }
      }
    }
    void poll()
    return () => { controller.abort(); clearTimeout(timer) }
  }, [revision])

  async function sync(category: string, full: boolean) {
    if (mutation.current) return
    const baseline = snapshot?.status.favorites.downloads?.baseline_state
    const controller = new AbortController()
    mutation.current = controller
    setPending(true)
    setRequestError('')
    setNotice('')
    try {
      await syncFavorites(category, full, controller.signal)
      if (!controller.signal.aborted) setNotice(baseline && baseline !== 'ready'
        ? 'Baseline sync requested for all ten categories.'
        : `${full ? 'Full re-sync' : 'Sync'} requested for ${category === 'all' ? 'all categories' : `category ${category}`}.`)
    } catch (error) {
      if (!controller.signal.aborted) setRequestError((error as Error).message)
    } finally {
      mutation.current = null
      if (!controller.signal.aborted) {
        setPending(false)
        setRevision((value) => value + 1)
      }
    }
  }

  const error = statusError || (connection?.error ? collectorMessages[connection.error] ?? 'Collector status is unavailable.' : '')
  return {
    connection, snapshot, checking, pending, notice, error, requestError, revision,
    stale: !!snapshot && (!!error || !connection?.status),
    connectionUnknown: !!statusError,
    disabled: pending || !!error || !connection?.status,
    refresh: () => setRevision((value) => value + 1),
    sitemapChanged: (sitemap: SitemapStatus) => {
      setSnapshot((current) => current ? { ...current, status: { ...current.status, sitemap } } : current)
      setRevision((value) => value + 1)
    },
    downloadSettingsSaved: (categories: number[]) => {
      setSnapshot((current) => {
        if (!current?.status.favorites.downloads) return current
        return { ...current, status: { ...current.status, favorites: { ...current.status.favorites,
          downloads: { ...current.status.favorites.downloads, categories },
        } } }
      })
      setRevision((value) => value + 1)
    },
    sync,
  }
}
