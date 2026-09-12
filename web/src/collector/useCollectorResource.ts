import { useEffect, useState } from 'react'

export function useCollectorResource<T>(load: (signal: AbortSignal) => Promise<T>, available: boolean, refreshKey: number, interval = 5000) {
  const [snapshot, setSnapshot] = useState<{ data: T, at: string } | null>(null)
  const [error, setError] = useState('')
  const [checking, setChecking] = useState(false)
  const [revision, setRevision] = useState(0)

  useEffect(() => {
    if (!available) return
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function poll() {
      setChecking(true)
      try {
        const data = await load(controller.signal)
        if (!controller.signal.aborted) {
          setSnapshot({ data, at: new Date().toISOString() })
          setError('')
        }
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
  }, [load, available, refreshKey, revision, interval])

  return {
    data: snapshot?.data, at: snapshot?.at, error, checking,
    stale: !!snapshot && (!available || !!error),
    refresh: () => setRevision((value) => value + 1),
    update: (data: T) => { setSnapshot({ data, at: new Date().toISOString() }); setRevision((value) => value + 1) },
  }
}
