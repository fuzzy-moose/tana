import { Button, Text } from '@radix-ui/themes'
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
    <Text as="p" size="2" role="status">Main background metadata collection is {paused ? 'paused' : 'enabled'}.</Text>
    <Text as="p" size="1" color="gray">Pauses background metadata retrieval and reference-import validation on the main worker. Explicit requests and proxy collection continue.</Text>
    {paused && <Text as="p" size="1" color="gray">Assigned batches may still finish. Paused until you resume, including after collector restarts.</Text>}
    <Button size="2" variant="soft" type="button" disabled={!available || pending} onClick={() => void update()} color="gray">
      {pending ? 'Saving…' : paused ? 'Resume background metadata' : 'Pause background metadata'}
    </Button>
    {error && <Text as="p" size="2" color="red" role="alert">{error}</Text>}
  </>
}
