import { useEffect, useRef, useState } from 'react'
import { APIError } from '../api'

export function useCollectorResource<T>(load: (signal: AbortSignal) => Promise<T>, available: boolean, refreshKey: number, interval = 5000,
  { rescheduleOnIntervalChange = false }: { rescheduleOnIntervalChange?: boolean } = {}) {
  const [snapshot, setSnapshot] = useState<{ data: T, at: string } | null>(null)
  const [error, setError] = useState('')
  const [errorCode, setErrorCode] = useState('')
  const [checking, setChecking] = useState(false)
  const [revision, setRevision] = useState(0)
  const intervalRef = useRef(interval)
  const rescheduleRef = useRef<(() => void) | null>(null)
  // By default a new interval loads at once; with rescheduleOnIntervalChange it only moves the pending poll.
  const intervalKey = rescheduleOnIntervalChange ? 0 : interval

  useEffect(() => {
    if (!available) return
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout> | undefined
    const schedule = () => { clearTimeout(timer); timer = setTimeout(poll, intervalRef.current) }
    rescheduleRef.current = () => { if (timer !== undefined) schedule() }
    async function poll() {
      timer = undefined
      setChecking(true)
      try {
        const data = await load(controller.signal)
        if (!controller.signal.aborted) {
          setSnapshot({ data, at: new Date().toISOString() })
          setError('')
          setErrorCode('')
        }
      } catch (error) {
        if (!controller.signal.aborted) {
          setError((error as Error).message)
          setErrorCode(error instanceof APIError ? error.code ?? '' : '')
        }
      } finally {
        if (!controller.signal.aborted) {
          setChecking(false)
          schedule()
        }
      }
    }
    void poll()
    return () => { controller.abort(); clearTimeout(timer); rescheduleRef.current = null }
  }, [load, available, refreshKey, revision, intervalKey])

  useEffect(() => {
    intervalRef.current = interval
    rescheduleRef.current?.()
  }, [interval])

  return {
    data: snapshot?.data, at: snapshot?.at, error, errorCode, checking,
    stale: !!snapshot && (!available || !!error),
    refresh: () => setRevision((value) => value + 1),
    update: (data: T) => { setSnapshot({ data, at: new Date().toISOString() }); setRevision((value) => value + 1) },
  }
}
