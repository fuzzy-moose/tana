import { useEffect, useRef, useState } from 'react'
import { listLibraries } from '../libraries/api'
import type { Library } from '../libraries/api'
import { changeLibraryDelivery, deliveryActive, listLibraryDeliveries, startLibraryDelivery } from './libraryDeliveries'
import type { LibraryDeliveryAction, LibraryDeliveryBatch, LibraryDeliveryRequest } from './libraryDeliveries'

export function useLibraryDeliveries(refreshKey: number) {
  const [batches, setBatches] = useState<LibraryDeliveryBatch[] | null>(null)
  const [libraries, setLibraries] = useState<Library[] | null>(null)
  const [error, setError] = useState('')
  const [libraryError, setLibraryError] = useState('')
  const [requestError, setRequestError] = useState('')
  const [pending, setPending] = useState(false)
  const [checking, setChecking] = useState(true)
  const [revision, setRevision] = useState(0)
  const mutation = useRef<AbortController | null>(null)

  useEffect(() => () => mutation.current?.abort(), [])
  useEffect(() => {
    const controller = new AbortController()
    void listLibraries(controller.signal).then((result) => {
      if (controller.signal.aborted) return
      if (!Array.isArray(result)) throw new Error('Tana returned an unexpected library list.')
      setLibraries(result)
      setLibraryError('')
    }).catch((error: Error) => { if (!controller.signal.aborted) setLibraryError(error.message) })
    return () => controller.abort()
  }, [refreshKey, revision])

  useEffect(() => {
    if (pending) return
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function poll() {
      setChecking(true)
      try {
        const result = await listLibraryDeliveries(controller.signal)
        if (controller.signal.aborted) return
        if (!Array.isArray(result.batches)) throw new Error('Tana returned an unexpected delivery history.')
        setBatches(result.batches)
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
  }, [refreshKey, revision, pending])

  async function perform(operation: (signal: AbortSignal) => Promise<LibraryDeliveryBatch>) {
    if (mutation.current) return false
    const controller = new AbortController()
    mutation.current = controller
    setPending(true)
    setRequestError('')
    try {
      const result = await operation(controller.signal)
      if (controller.signal.aborted) return false
      setBatches((current) => [result, ...(current ?? []).filter((batch) => batch.id !== result.id)].sort((a, b) => b.id - a.id))
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

  return {
    batches: batches ?? [], libraries: libraries ?? [], loaded: batches !== null, librariesLoaded: libraries !== null,
    active: batches?.find(deliveryActive), pending, checking, error, libraryError, requestError,
    disabled: pending || !!error || batches === null,
    refresh: () => setRevision((value) => value + 1),
    start: (body: LibraryDeliveryRequest) => perform((signal) => startLibraryDelivery(body, signal)),
    change: (id: number, action: LibraryDeliveryAction) => perform((signal) => changeLibraryDelivery(id, action, signal)),
  }
}
