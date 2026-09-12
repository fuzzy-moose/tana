import { useEffect, useRef, useState } from 'react'
import { getInventoryStatus } from '../collector/api'
import { useCollectorResource } from '../collector/useCollectorResource'
import { getFeedStatus, refreshFeed } from './api'
import type { FeedStatus } from './api'

export function usePandaCollection() {
  const [feed, setFeed] = useState<FeedStatus | null>(null)
  const [feedError, setFeedError] = useState('')
  const [requestError, setRequestError] = useState('')
  const [pending, setPending] = useState(false)
  const [revision, setRevision] = useState(0)
  const inventory = useCollectorResource(getInventoryStatus, true, revision, 30000)
  const mutation = useRef<AbortController | null>(null)

  useEffect(() => () => mutation.current?.abort(), [])
  useEffect(() => {
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function poll() {
      try {
        const next = await getFeedStatus(controller.signal)
        if (!controller.signal.aborted) {
          setFeed(next)
          setFeedError('')
        }
      } catch (error) {
        if (!controller.signal.aborted) setFeedError((error as Error).message)
      } finally {
        if (!controller.signal.aborted) timer = setTimeout(poll, 5000)
      }
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

  return { feed, inventory: inventory.data, error: feedError || inventory.error, requestError, pending, capture, retry: () => setRevision((value) => value + 1) }
}
