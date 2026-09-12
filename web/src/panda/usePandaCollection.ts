import { useEffect, useRef, useState } from 'react'
import { collectorMessages, getCollectorStatus } from '../collector/api'
import type { CollectorStatus } from '../collector/api'
import { getFeedStatus, refreshFeed } from './api'
import type { FeedStatus } from './api'

export function usePandaCollection() {
  const [feed, setFeed] = useState<FeedStatus | null>(null)
  const [inventory, setInventory] = useState<CollectorStatus['inventory'] | null>(null)
  const [error, setError] = useState('')
  const [requestError, setRequestError] = useState('')
  const [pending, setPending] = useState(false)
  const [revision, setRevision] = useState(0)
  const mutation = useRef<AbortController | null>(null)

  useEffect(() => () => mutation.current?.abort(), [])
  useEffect(() => {
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function poll() {
      const [feedResult, connectionResult] = await Promise.allSettled([
        getFeedStatus(controller.signal), getCollectorStatus(controller.signal),
      ])
      if (controller.signal.aborted) return
      const errors: string[] = []
      if (feedResult.status === 'fulfilled') setFeed(feedResult.value)
      else errors.push((feedResult.reason as Error).message)
      if (connectionResult.status === 'fulfilled') {
        const connection = connectionResult.value
        if (connection.status) setInventory(connection.status.inventory)
        else errors.push(collectorMessages[connection.error ?? 'collector_status_unavailable'] ?? 'Collector status is unavailable.')
      } else errors.push((connectionResult.reason as Error).message)
      setError(errors[0] ?? '')
      timer = setTimeout(poll, 5000)
    }
    void poll()
    return () => { controller.abort(); clearTimeout(timer) }
  }, [revision])

  async function capture() {
    if (mutation.current) return
    const controller = new AbortController()
    mutation.current = controller
    setPending(true)
    setRequestError('')
    try {
      const next = await refreshFeed(controller.signal)
      if (!controller.signal.aborted) setFeed(next)
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

  return { feed, inventory, error, requestError, pending, capture, retry: () => setRevision((value) => value + 1) }
}
