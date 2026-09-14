import { Button, Card, Checkbox, Flex, Grid, Heading, Text } from '@radix-ui/themes'
import { useEffect, useRef, useState } from 'react'
import { saveFavoriteDownloadSettings } from './api'
import type { FavoriteCategory, FavoriteDownloadsStatus } from './api'

export default function FavoriteDownloads({ status, categories, available, onSaved }: {
  status: FavoriteDownloadsStatus
  categories: FavoriteCategory[]
  available: boolean
  onSaved: (categories: number[]) => void
}) {
  const [draft, setDraft] = useState<number[] | null>(null)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const mutation = useRef<AbortController | null>(null)
  useEffect(() => () => mutation.current?.abort(), [])
  const selected = draft ?? status.categories

  async function save() {
    if (mutation.current) return
    const controller = new AbortController()
    mutation.current = controller
    setPending(true)
    setError('')
    setNotice('')
    try {
      const result = await saveFavoriteDownloadSettings(selected, controller.signal)
      if (!controller.signal.aborted) {
        onSaved(result.categories)
        setDraft(null)
        setNotice('Download categories saved.')
      }
    } catch (error) {
      if (!controller.signal.aborted) setError((error as Error).message)
    } finally {
      mutation.current = null
      if (!controller.signal.aborted) setPending(false)
    }
  }

  return <Card asChild><div><Flex direction="column" gap="3">
    <Heading as="h3" size="3">Automatic favorite downloads</Heading>
    <Text as="p" size="2">{status.baseline_state === 'not_started'
      ? 'Your first sync will establish a baseline across all ten categories without downloading existing favorites.'
      : status.baseline_state === 'collecting'
        ? `Establishing baseline: ${status.baseline_categories} of 10 categories complete. No automatic downloads until a later sync. Use Sync favorites to resume failed categories.`
        : 'Baseline complete. Future syncs download newly discovered favorites in the selected categories.'}</Text>
    <form onSubmit={(event) => { event.preventDefault(); if (available && !pending) void save() }}>
      <Grid columns={{ initial: '1', xs: '2', md: '5' }} gap="3" my="3" asChild><fieldset disabled={!available || pending}>
        <legend>Download new favorites from</legend>
        {categories.map((category) => <Flex align="center" gap="2" key={category.category} asChild><label>
          <Checkbox checked={selected.includes(category.category)} onCheckedChange={(checked) => {
            setDraft((checked === true) ? [...selected, category.category].sort((a, b) => a - b) : selected.filter((id) => id !== category.category))
            setNotice('')
          }} />
          {category.name ? `${category.category} · ${category.name}` : `Category ${category.category}`}
        </label></Flex>)}
      </fieldset></Grid>
      <Text as="p" size="1" color="gray">Enabling a category does not download previously observed favorites. Disabling it leaves queued downloads running. Archives stay in the collector.</Text>
      <Button size="2" variant="soft" type="submit" disabled={!available || pending || draft === null} color="gray">{pending ? 'Saving…' : 'Save download categories'}</Button>
      {notice && <Text as="p" size="2" color="green" role="status">{notice}</Text>}
      {error && <Text as="p" size="2" color="red" role="alert">{error}</Text>}
    </form>
  </Flex></div></Card>
}
