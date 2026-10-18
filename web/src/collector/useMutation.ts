import { useEffect, useRef, useState } from 'react'

interface MutationOptions<T> {
  /** Runs only if the mutation was not aborted; a returned string becomes the notice. */
  onSuccess?: (result: T) => string | void
  /** Runs after success or failure, unless the mutation was aborted. */
  onSettled?: () => void
}

/**
 * Runs one request at a time and aborts it on unmount. State updates are skipped for aborted requests.
 * `run` resolves to whether the request succeeded.
 */
export function useMutation() {
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const current = useRef<AbortController | null>(null)
  useEffect(() => () => current.current?.abort(), [])

  async function run<T>(operation: (signal: AbortSignal) => Promise<T>, { onSuccess, onSettled }: MutationOptions<T> = {}) {
    if (current.current) return false
    const controller = new AbortController()
    current.current = controller
    setPending(true)
    setError('')
    setNotice('')
    try {
      const result = await operation(controller.signal)
      if (controller.signal.aborted) return false
      setNotice(onSuccess?.(result) || '')
      return true
    } catch (error) {
      if (!controller.signal.aborted) setError((error as Error).message)
      return false
    } finally {
      current.current = null
      if (!controller.signal.aborted) {
        setPending(false)
        onSettled?.()
      }
    }
  }

  return { pending, error, notice, setNotice, run, clear: () => { setError(''); setNotice('') } }
}
