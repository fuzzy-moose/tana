import { Badge, Button, Card, Flex, Heading, Link, Popover, Progress, Select, Text } from '@radix-ui/themes'
import { deliveryActive, deliveryItemState } from './libraryDeliveries'
import type { useLibraryDeliveries } from './useLibraryDeliveries'

export default function LibraryDeliveries({ deliveries, libraryID, onLibraryChange, canStart, completedCount }: {
  deliveries: ReturnType<typeof useLibraryDeliveries>
  libraryID: number
  onLibraryChange: (id: number) => void
  canStart: boolean
  completedCount: number
}) {
  return <Flex direction="column" gap="2" minWidth="0" asChild><section aria-label="Add downloads to a library">
    <Flex align="end" wrap="wrap" gap="2">
      <Flex direction="column" gap="2" flexGrow="1" minWidth="0" asChild><label>Destination library
        <Select.Root value={libraryID ? String(libraryID) : ''} onValueChange={(value) => onLibraryChange(Number(value))} disabled={deliveries.pending || !!deliveries.active || !deliveries.librariesLoaded || !!deliveries.libraryError}><Select.Trigger aria-label="Destination library" placeholder="Choose a library" /><Select.Content>

          {deliveries.libraries.map((library) => <Select.Item key={library.id} value={String(library.id)}>{library.name}{library.availability === 'unavailable' ? ' (unavailable)' : ''}</Select.Item>)}
        </Select.Content></Select.Root>
      </label></Flex>
      <Button size="2" variant="solid" type="button" disabled={!canStart || completedCount === 0} onClick={() => void deliveries.start({ library_id: libraryID, all: true })}>Add all</Button>
      <Button size="2" variant="soft" color="gray" type="button" disabled={deliveries.pending || deliveries.checking} onClick={deliveries.refresh}>Refresh deliveries</Button>
    </Flex>
    <Flex align="center" gap="2" wrap="wrap">
      <Text size="1" color="gray">Successful imports remove collector copies.</Text>
      <Popover.Root>
        <Popover.Trigger><Button size="1" variant="ghost" color="gray">Delivery details</Button></Popover.Trigger>
        <Popover.Content maxWidth="360px">
          <Flex direction="column" gap="3">
            <Heading as="h3" size="3">Add downloads to a library</Heading>
            <Text as="p" size="2">Add all includes every completed download when clicked, across all pages and filters. Archives transfer one at a time; collector copies are removed after successful import.</Text>
            <Text as="p" size="2">Delivery continues when you close this page. Successful batches disappear automatically.</Text>
          </Flex>
        </Popover.Content>
      </Popover.Root>
    </Flex>
    {deliveries.librariesLoaded && deliveries.libraries.length === 0 && <Text as="p" size="1" color="gray"><Link href="#/libraries">Register a library</Link> to add downloads.</Text>}
    {deliveries.active && <Text as="p" size="2" color="green" role="status">One delivery batch at a time. Finish or stop the active batch to start another.</Text>}
    {deliveries.libraryError && <Text as="p" size="2" color="red" role="alert">{deliveries.libraryError}</Text>}
    {deliveries.requestError && <Text as="p" size="2" color="red" role="alert">{deliveries.requestError}</Text>}
    {deliveries.error && <Text as="p" size="2" color="red" role="alert">{deliveries.error} Delivery progress may be stale.</Text>}
    {!deliveries.loaded && !deliveries.error && <Text as="p" size="1" color="gray">Loading deliveries…</Text>}
    {deliveries.batches.map((batch) => {
      const active = deliveryActive(batch)
      const done = batch.items.filter((item) => ['completed', 'skipped', 'failed', 'cleanup_pending'].includes(item.state)).length
      const added = batch.items.filter((item) => ['completed', 'cleanup_pending'].includes(item.state)).length
      const failed = batch.items.filter((item) => item.state === 'failed').length
      const cleanup = batch.items.filter((item) => item.state === 'cleanup_pending').length
      const cleanupExhausted = batch.items.some((item) => item.state === 'cleanup_pending' && item.cleanup_attempts>= 3)
      const skipped = batch.items.filter((item) => item.state === 'skipped').length
      const disabled = deliveries.disabled
      const retryDisabled = disabled || !!deliveries.active
      const library = deliveries.libraries.find((library) => library.id === batch.library_id)
      const current = batch.items.find((item) => ['transferring', 'transferred', 'saved', 'importing'].includes(item.state))
      const state = { running: 'Delivering', paused: 'Paused', stopped: 'Stopped', completed: 'Completed', completed_with_errors: 'Needs attention' }[batch.state]
      return <Card key={batch.id} asChild><article aria-label={`Delivery ${batch.id}`}><Flex direction="column" gap="3">
        <Flex align="center" justify="between" wrap="wrap" gap="3">
          <Heading as="h4" size="3">Delivery {batch.id} · {library?.name ?? `Library ${batch.library_id}`}</Heading>
          <Badge color="gray">{batch.stop_requested && batch.state === 'running' ? 'Stopping after current archive' : state}</Badge>
        </Flex>
        <Text as="p" size="2">{done} of {batch.items.length} processed · {added} added · {skipped} already in library · {failed} failed{cleanup > 0 && ` · ${cleanup} awaiting collector cleanup`}</Text>
        <Progress aria-label={`Delivery ${batch.id} progress`} value={done} max={batch.items.length || 1} />
        {current && <Text as="p" size="1" color="gray">Gallery {current.gallery_id} · {deliveryItemState(current)}</Text>}
        {batch.error && <Text as="p" size="2" color="red">{batch.error}</Text>}
        {batch.state === 'paused' && <Text as="p" size="1" color="gray">Progress is saved. Resume explicitly when the destination is available.</Text>}
        {batch.state === 'stopped' && <Text as="p" size="1" color="gray">Remaining archives were left on the collector. Retry remaining to continue this batch.</Text>}
        <Flex align="center" wrap="wrap" gap="2">
          {batch.state === 'paused' && <Button size="2" variant="soft" disabled={disabled} onClick={() => void deliveries.change(batch.id, 'resume')} color="gray">Resume</Button>}
          {active && <Button size="2" variant="soft" disabled={disabled || batch.state === 'running' && batch.stop_requested} onClick={() => void deliveries.change(batch.id, 'stop')} color="gray">{batch.state === 'paused' ? 'Stop batch' : 'Stop after current archive'}</Button>}
          {!active && (failed > 0 || batch.state === 'stopped' && batch.items.some((item) => ['queued', 'transferring', 'transferred', 'saved', 'importing'].includes(item.state))) && <Button size="2" variant="soft" disabled={retryDisabled} onClick={() => void deliveries.change(batch.id, 'retry')} color="gray">{batch.state === 'stopped' ? 'Retry remaining' : 'Retry failed'}</Button>}
          {cleanupExhausted && <Button size="2" variant="soft" disabled={disabled} onClick={() => void deliveries.change(batch.id, 'retry-cleanup')} color="gray">Retry cleanup</Button>}
        </Flex>
        <details className="library-delivery-items">
          <summary>Gallery results ({batch.items.length})</summary>
          <Flex direction="column" gap="2" asChild><ul style={{ listStyle: 'none', padding: 0, margin: 0 }}>{batch.items.map((item) => <Flex direction="column" gap="1" key={item.gallery_id} asChild><li>
            <strong>Gallery {item.gallery_id} · {deliveryItemState(item)}</strong>
            {item.error && item.state !== 'skipped' && <span>{item.error}</span>}
          </li></Flex>)}</ul></Flex>
        </details>
      </Flex></article></Card>
    })}
  </section></Flex>
}
