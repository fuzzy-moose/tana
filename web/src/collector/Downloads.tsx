import { Badge, Button, Card, Flex, Grid, Heading, Popover, Table, Text, TextField } from '@radix-ui/themes'
import { useState } from 'react'
import { downloadFailure, downloadFileURL, downloadPageSize, downloadState } from './downloads'
import type { DownloadFilter } from './downloads'
import { useDownloads } from './useDownloads'
import { useLibraryDeliveries } from './useLibraryDeliveries'
import LibraryDeliveries from './LibraryDeliveries'
import { deliveryActive } from './libraryDeliveries'

const filters: { state: DownloadFilter, label: string }[] = [
  { state: '', label: 'All' },
  { state: 'running', label: 'Active' },
  { state: 'queued', label: 'Pending' },
  { state: 'completed', label: 'Completed' },
  { state: 'failed', label: 'Failed' },
  { state: 'cancelled', label: 'Cancelled' },
  { state: 'deleting', label: 'Deleting' },
]

function date(value: string) { return new Date(value).toLocaleString() }
function size(bytes: number) {
  if (bytes < 1024) return `${bytes.toLocaleString()} B`
  if (bytes < 1024 ** 2) return `${(bytes / 1024).toFixed(1)} KiB`
  if (bytes < 1024 ** 3) return `${(bytes / 1024 ** 2).toFixed(1)} MiB`
  return `${(bytes / 1024 ** 3).toFixed(1)} GiB`
}

export default function Downloads({ available, refreshKey }: { available: boolean, refreshKey: number }) {
  const downloads = useDownloads(available, refreshKey)
  const deliveries = useLibraryDeliveries(refreshKey)
  const [url, setURL] = useState('')
  const [deleting, setDeleting] = useState<number | null>(null)
  const [libraryID, setLibraryID] = useState(0)
  const actionsDisabled = downloads.disabled || downloads.stale || downloads.checking || !downloads.loaded
  const canDeliver = !actionsDisabled && !deliveries.disabled && !deliveries.active && !deliveries.libraryError && deliveries.libraries.some((library) => library.id === libraryID)
  const deliveryOwnsArchive = (id: number) => deliveries.batches.some((batch) => batch.items.some((item) => item.gallery_id === id && (
    item.state === 'cleanup_pending' || deliveryActive(batch) && !['completed', 'skipped', 'failed'].includes(item.state)
  )))
  const filterLabel = filters.find((filter) => filter.state === downloads.filter)!.label
  const total = downloads.counts ? Object.values(downloads.counts).reduce((sum, count) => sum + count, 0) : undefined
  const storage = downloads.storage

  return (
    <Flex direction="column" gap="4" asChild><section aria-label="Download queue">
      {storage?.paused && <Text as="p" size="2" color="green" role="status">
        {storage.reason === 'space_check_failed'
          ? 'Downloads paused: the collector cannot check available storage.'
          : 'Downloads paused: collector storage is low.'}
        {' '}{storage.available_bytes === null ? 'Free space unknown.' : `${size(storage.available_bytes)} free.`}
        {' '}Downloads resume automatically at {size(storage.resume_at_bytes)} free. Completed downloads can still be added to a library.
      </Text>}
      <Grid columns={{ initial: '1', lg: '2' }} gap="4" align="start">
      <Flex direction="column" gap="2" minWidth="0">
      <Flex align="end" wrap="wrap" gap="2" asChild><form onSubmit={async (event) => {
        event.preventDefault()
        if (await downloads.submit(url)) setURL('')
      }}>
        <Flex direction="column" gap="2" flexGrow="1" minWidth="0" asChild><label>Panda gallery URL
          <TextField.Root size="2" type="url" required value={url} onChange={(event) => setURL(event.target.value)} disabled={downloads.disabled} placeholder="Paste a gallery URL" autoComplete="off" />
        </label></Flex>
        <Button size="2" variant="solid" type="submit" disabled={downloads.disabled || !url.trim()}>{downloads.pending ? 'Requesting…' : 'Add download'}</Button>
        <Button size="2" variant="soft" color="gray" type="button" disabled={!available || downloads.checking || downloads.pending} onClick={downloads.refresh}>Refresh downloads</Button>
      </form></Flex>
      <Flex align="center" gap="2">
        <Text size="1" color="gray">Original archives are retained on the collector.</Text>
        <Popover.Root>
          <Popover.Trigger><Button size="1" variant="ghost" color="gray">Download details</Button></Popover.Trigger>
          <Popover.Content maxWidth="360px">
            <Flex direction="column" gap="3">
              <Heading as="h3" size="3">Download original archives</Heading>
              <Text as="p" size="2">Submitting the same gallery reuses its existing download. Failed or cancelled jobs can be retried below.</Text>
              <Text as="p" size="2">Save ZIP copies an archive to your device and keeps the collector copy. Add to library transfers it into Tana.</Text>
            </Flex>
          </Popover.Content>
        </Popover.Root>
      </Flex>
      </Flex>
      <LibraryDeliveries deliveries={deliveries} libraryID={libraryID} onLibraryChange={setLibraryID} canStart={canDeliver} completedCount={downloads.counts?.completed ?? 0} />
      </Grid>
      <Flex align="center" wrap="wrap" gap="2" role="group" aria-label="Filter downloads">
        {filters.map(({ state, label }) => <Button size="2" variant="soft" key={state} type="button" aria-pressed={downloads.filter === state} color={downloads.filter === state ? 'green' : 'gray'} disabled={downloads.disabled} onClick={() => { downloads.selectFilter(state); setDeleting(null) }}>
          <span>{label}</span>{' '}
          <strong>{(state ? downloads.counts?.[state] : total)?.toLocaleString() ?? '—'}</strong>
        </Button>)}
      </Flex>
      {downloads.filter === 'queued' && <Text as="p" size="1" color="gray">Pending downloads are queued or waiting for storage space, an automatic retry, or Panda cooldown.</Text>}
      {downloads.notice && <Text as="p" size="2" color="green" role="status">{downloads.notice}</Text>}
      {downloads.requestError && <Text as="p" size="2" color="red" role="alert">{downloads.requestError}</Text>}
      {downloads.error && <Text as="p" size="2" color="red" role="alert">{downloads.error}</Text>}
      {downloads.stale && <Text as="p" size="2" color="green" role="status">Download status is stale. Actions will be available when the collector reconnects and the list refreshes.</Text>}
      {!downloads.loaded && !downloads.error && <Text as="p" size="1" color="gray">{available ? 'Loading downloads…' : 'Downloads are unavailable while the collector is disconnected.'}</Text>}
      {downloads.loaded && downloads.jobs.length === 0 && <Text as="p" size="2" role="status">{downloads.filter
        ? `No ${filterLabel.toLowerCase()} downloads.`
        : 'No downloads yet. Paste a Panda gallery URL to get started.'}</Text>}
      {downloads.jobs.length > 0 && <div className="collector-table-scroll">
        <Table.Root size="1" variant="surface" className="collector-table collector-downloads">
          <Text size="1" color="gray" align="left" mb="2" asChild><caption>{filterLabel} downloads · newest first</caption></Text>
          <Table.Header><Table.Row><Table.ColumnHeaderCell scope="col">Gallery</Table.ColumnHeaderCell><Table.ColumnHeaderCell scope="col">State</Table.ColumnHeaderCell><Table.ColumnHeaderCell scope="col">Archive size</Table.ColumnHeaderCell><Table.ColumnHeaderCell scope="col">Dates</Table.ColumnHeaderCell><Table.ColumnHeaderCell scope="col">Actions</Table.ColumnHeaderCell></Table.Row></Table.Header>
          <Table.Body>{downloads.jobs.map((job) => <Table.Row key={job.gallery_id}>
            <Table.RowHeaderCell scope="row">Gallery {job.gallery_id}</Table.RowHeaderCell>
            <Table.Cell><Badge color={job.state === 'failed' ? 'red' : job.state === 'completed' ? 'green' : job.state === 'running' ? 'blue' : 'gray'}>{downloadState(job)}</Badge>
              {job.error && <Text as="span" size="1" color="gray" style={{ display: 'block' }} mt="1">{downloadFailure(job.error)}</Text>}
              {job.failures > 0 && <Text as="span" size="1" color="gray" style={{ display: 'block' }} mt="1">{job.failures} failed {job.failures === 1 ? 'attempt' : 'attempts'}</Text>}
              {job.retry_at && <Text as="span" size="1" color="gray" style={{ display: 'block' }} mt="1">Retry after {date(job.retry_at)}</Text>}
            </Table.Cell>
            <Table.Cell>{job.state === 'completed' ? size(job.size_bytes) : job.expected_size_bytes ? size(job.expected_size_bytes) : '—'}</Table.Cell>
            <Table.Cell><span>Added {date(job.created_at)}</span><Text as="span" size="1" color="gray" style={{ display: 'block' }} mt="1">Updated {date(job.updated_at)}</Text></Table.Cell>
            <Table.Cell>
              <Flex align="center" wrap="wrap" gap="2">
                {job.state === 'completed' && <Button size="2" variant="solid" disabled={!canDeliver || deliveryOwnsArchive(job.gallery_id)} onClick={() => void deliveries.start({ library_id: libraryID, gallery_id: job.gallery_id })}>Add to library</Button>}
                {job.state === 'completed' && (actionsDisabled
                  ? <Button size="2" variant="soft" disabled color="gray">Save ZIP</Button>
                  : <Button size="2" variant="soft" asChild color="gray"><a href={downloadFileURL(job.gallery_id)} download>Save ZIP</a></Button>)}
                {(job.state === 'queued' || job.state === 'running') && <Button size="2" variant="soft" disabled={actionsDisabled} onClick={() => void downloads.change(job.gallery_id, 'cancel')} color="gray">Cancel</Button>}
                {(job.state === 'failed' || job.state === 'cancelled') && <Button size="2" variant="soft" disabled={actionsDisabled} onClick={() => void downloads.change(job.gallery_id, 'retry')} color="gray">Retry</Button>}
                {['completed', 'failed', 'cancelled', 'deleting'].includes(job.state) && <Button size="2" variant="soft" disabled={actionsDisabled || deliveryOwnsArchive(job.gallery_id)} onClick={() => setDeleting(job.gallery_id)} color="red">Delete</Button>}
              </Flex>
              {deleting === job.gallery_id && <Card asChild><div role="group" aria-label={`Delete download for gallery ${job.gallery_id}`}><Flex direction="column" gap="3">
                <Text as="p" size="2">Delete this download and any archive retained on the collector?</Text>
                <Flex align="center" wrap="wrap" gap="2">
                  <Button size="2" variant="soft" disabled={actionsDisabled || deliveryOwnsArchive(job.gallery_id)} onClick={async () => { if (await downloads.change(job.gallery_id, 'delete')) setDeleting(null) }} color="red">Delete download</Button>
                  <Button size="2" variant="soft" disabled={downloads.pending} onClick={() => setDeleting(null)} color="gray">Keep download</Button>
                </Flex>
              </Flex></div></Card>}
            </Table.Cell>
          </Table.Row>)}</Table.Body>
        </Table.Root>
      </div>}
      {(downloads.offset > 0 || downloads.hasNext) && <Flex align="center" justify="end" wrap="wrap" gap="3" asChild><nav aria-label="Download pages">
        <Button size="2" variant="soft" disabled={actionsDisabled || downloads.checking || downloads.offset === 0} onClick={downloads.previous} color="gray">Previous downloads</Button>
        <span>Page {downloads.offset / downloadPageSize + 1}</span>
        <Button size="2" variant="soft" disabled={actionsDisabled || downloads.checking || !downloads.hasNext} onClick={downloads.next} color="gray">Next downloads</Button>
      </nav></Flex>}
    </section></Flex>
  )
}
