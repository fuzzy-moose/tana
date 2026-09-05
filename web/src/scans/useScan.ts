import { useEffect, useRef, useState } from 'react'
import { request } from '../api'

export interface ScanStatus {
  phase: 'idle' | 'discovering' | 'importing' | 'completed' | 'completed_with_errors' | 'failed'
  library_id?: string
  libraries_total: number
  discovered: number
  imported: number
  failed_sources: number
  skipped: number
  discovery_errors: number
  galleries_created: number
  started_at: string | null
  finished_at: string | null
}

export function useScan() {
  const [status, setStatus] = useState<ScanStatus | null>(null)
  const [statusError, setStatusError] = useState('')
  const [requestError, setRequestError] = useState('')
  const [pending, setPending] = useState(false)
  const [refresh, setRefresh] = useState(0)
  const mutation = useRef<AbortController | null>(null)
  useEffect(() => () => mutation.current?.abort(), [])

  useEffect(() => {
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function poll() {
      try {
        const next = await request<ScanStatus>('/api/scans/status', { signal: controller.signal })
        if (!controller.signal.aborted) { setStatus(next); setStatusError('') }
      } catch (error) {
        if (!controller.signal.aborted) setStatusError((error as Error).message)
      } finally {
        if (!controller.signal.aborted) timer = setTimeout(poll, 1000)
      }
    }
    void poll()
    return () => { controller.abort(); clearTimeout(timer) }
  }, [refresh])

  async function start(libraryID?: string) {
    const controller = new AbortController()
    mutation.current = controller
    setPending(true)
    setRequestError('')
    try {
      await request('/api/scans', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(libraryID ? { library_id: libraryID } : {}),
        signal: controller.signal,
      }, {
        scan_active: 'A scan is already running. Its status is shown below.',
        not_found: 'This library no longer exists. Refresh the library list.',
      })
      if (!controller.signal.aborted) setStatus(null)
    } catch (error) {
      if (!controller.signal.aborted) setRequestError((error as Error).message)
    } finally {
      if (!controller.signal.aborted) {
        setPending(false)
        setRefresh((n) => n + 1)
      }
    }
  }
  const active = status?.phase === 'discovering' || status?.phase === 'importing'
  return { status, error: requestError || statusError, disabled: pending || active || !status || !!statusError, start }
}

export type ScanControls = ReturnType<typeof useScan>
