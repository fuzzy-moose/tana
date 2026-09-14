import { useEffect, useRef, useState } from 'react'
import { setMainBackgroundPaused } from './api'

export default function MetadataCollection({ paused, available, onChanged }: {
  paused: boolean
  available: boolean
  onChanged: (paused: boolean) => void
}) {
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const mutation = useRef<AbortController | null>(null)
  useEffect(() => () => mutation.current?.abort(), [])

  async function update() {
    if (mutation.current || !available) return
    const controller = new AbortController()
    mutation.current = controller
    setPending(true)
    setError('')
    try {
      await setMainBackgroundPaused(!paused, controller.signal)
      if (!controller.signal.aborted) onChanged(!paused)
    } catch (error) {
      if (!controller.signal.aborted) setError((error as Error).message)
    } finally {
      mutation.current = null
      if (!controller.signal.aborted) setPending(false)
    }
  }

  return <>
    <p role="status">Main background metadata collection is {paused ? 'paused' : 'enabled'}.</p>
    <p className="field-help">Pauses background metadata retrieval and reference-import validation on the main worker. Explicit requests and proxy collection continue.</p>
    {paused && <p className="field-help">Assigned batches may still finish. Paused until you resume, including after collector restarts.</p>}
    <button className="button" type="button" disabled={!available || pending} onClick={() => void update()}>
      {pending ? 'Saving…' : paused ? 'Resume background metadata' : 'Pause background metadata'}
    </button>
    {error && <p className="error-message" role="alert">{error}</p>}
  </>
}
