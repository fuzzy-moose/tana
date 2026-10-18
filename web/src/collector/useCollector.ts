import { useEffect, useState } from 'react'
import { collectorMessages, getCollectorStatus } from './api'
import type { ConnectionStatus } from './api'

export function useCollector() {
  const [connection, setConnection] = useState<ConnectionStatus | null>(null)
  const [statusError, setStatusError] = useState('')
  const [checking, setChecking] = useState(true)
  const [revision, setRevision] = useState(0)

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

  const error = statusError || (connection?.error ? collectorMessages[connection.error] ?? 'Collector status is unavailable.' : '')
  return {
    connection, checking, error, revision,
    connectionUnknown: !!statusError,
    refresh: () => setRevision((value) => value + 1),
  }
}
