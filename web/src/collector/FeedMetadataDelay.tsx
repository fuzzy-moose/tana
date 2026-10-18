import { Button, Flex, Text, TextField } from '@radix-ui/themes'
import { useEffect, useRef, useState } from 'react'
import { saveFeedMetadataDelay } from './api'

export default function FeedMetadataDelay({ hours, available, onSaved }: {
  hours: number
  available: boolean
  onSaved: (hours: number) => void
}) {
  const [draft, setDraft] = useState<string | null>(null)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const mutation = useRef<AbortController | null>(null)
  useEffect(() => () => mutation.current?.abort(), [])
  const value = draft ?? String(hours)
  const valid = value.trim() !== '' && Number.isInteger(Number(value)) && Number(value) >= 0 && Number(value) <= 8760

  async function save() {
    if (mutation.current || !available || !valid) return
    const controller = new AbortController()
    mutation.current = controller
    setPending(true)
    setError('')
    setNotice('')
    try {
      const result = await saveFeedMetadataDelay(Number(value), controller.signal)
      if (!controller.signal.aborted) {
        onSaved(result.feed_metadata_delay_hours)
        setDraft(null)
        setNotice('Feed metadata delay saved.')
      }
    } catch (error) {
      if (!controller.signal.aborted) setError((error as Error).message)
    } finally {
      mutation.current = null
      if (!controller.signal.aborted) setPending(false)
    }
  }

  return <Flex direction="column" gap="3" asChild><form onSubmit={(event) => { event.preventDefault(); void save() }}>
    <Text as="label" size="2" htmlFor="feed-metadata-delay">Feed metadata delay (hours)</Text>
    <TextField.Root id="feed-metadata-delay" type="number" min={0} max={8760} step={1} required
      value={value} disabled={!available || pending} onChange={(event) => { setDraft(event.target.value); setNotice('') }} />
    <Text as="p" size="1" color="gray">Wait after publication (or first sighting when unknown) before collecting metadata for feed discoveries. Applies to waiting galleries on all collector workers. Explicit requests remain immediate. Use 0 for no delay.</Text>
    <Button type="submit" size="2" variant="soft" color="gray" disabled={!available || pending || draft === null || !valid}>{pending ? 'Saving…' : 'Save feed metadata delay'}</Button>
    {notice && <Text as="p" size="2" color="green" role="status">{notice}</Text>}
    {error && <Text as="p" size="2" color="red" role="alert">{error}</Text>}
  </form></Flex>
}
