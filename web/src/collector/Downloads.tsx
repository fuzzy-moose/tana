import { AlertDialog, Button, Card, Flex, Grid, Heading, Popover, Table, Text, TextField, VisuallyHidden } from '@radix-ui/themes'
import { useState } from 'react'
import { collectorLink, useCollectorListParams } from '../navigation'
import { downloadFailure, downloadFileURL, downloadFilter, downloadPageSize, downloadState, downloadTotal } from './downloads'
import type { DownloadJob } from './downloads'
import { formatBytes } from './formatBytes'
import { useHeaderAside } from './headerAside'
import { useDownloads } from './useDownloads'
import { useLibraryDeliveries } from './useLibraryDeliveries'
import CollectorPagination from './CollectorPagination'
import { useCollectorPagination } from './useCollectorPagination'
import LibraryDeliveries from './LibraryDeliveries'
import { deliveryActive } from './libraryDeliveries'
import type { CollectorPageProps } from './pageProps'
import ResourceStatus from './ResourceStatus'
import RowActions from './RowActions'
import { detailLines } from './detailLines'
import StatusBadge from './StatusBadge'
import type { Tone } from './StatusBadge'
import TableCard from './TableCard'
import type { TableTab } from './TableCard'
import { formatDate } from './formatDate'

// The collector's state filter accepts one job state, so "Active" (queued + running) cannot be requested as a single filter.
// Running jobs get their own tab instead.
const filters = [
  { state: '', label: 'All' },
  { state: 'running', label: 'Running' },
  { state: 'queued', label: 'Pending' },
  { state: 'completed', label: 'Completed' },
  { state: 'failed', label: 'Failed', color: 'red' as const },
  { state: 'cancelled', label: 'Cancelled' },
  { state: 'deleting', label: 'Deleting' },
] as const

const tones: Record<DownloadJob['state'], Tone> = { queued: 'amber', running: 'blue', completed: 'green', failed: 'red', cancelled: 'gray', deleting: 'gray' }

function jobDetails(job: DownloadJob) {
  return detailLines([
    job.error && downloadFailure(job.error),
    job.failures > 0 && `${job.failures} failed ${job.failures === 1 ? 'attempt' : 'attempts'}`,
    job.retry_at && `Retry after ${formatDate(job.retry_at)}`,
  ])
}

export default function Downloads({ available, refreshKey, interval }: CollectorPageProps) {
  const params = useCollectorListParams()
  const filter = downloadFilter(params.state)
  const downloads = useDownloads(available, refreshKey, interval, params.page, filter)
  const deliveries = useLibraryDeliveries(refreshKey)
  const [url, setURL] = useState('')
  const [deleting, setDeleting] = useState<number | null>(null)
  const [lastDeleting, setLastDeleting] = useState(0)
  const [libraryID, setLibraryID] = useState(0)
  const actionsDisabled = downloads.disabled || downloads.stale || downloads.checking || !downloads.loaded
  const canDeliver = !actionsDisabled && !deliveries.disabled && !deliveries.active && !deliveries.libraryError && deliveries.libraries.some((library) => library.id === libraryID)
  const deliveryOwnsArchive = (id: number) => deliveries.batches.some((batch) => batch.items.some((item) => item.gallery_id === id && (
    item.state === 'cleanup_pending' || deliveryActive(batch) && !['completed', 'skipped', 'failed'].includes(item.state)
  )))
  const filterLabel = filters.find((item) => item.state === filter)!.label
  const counts = downloads.counts
  const storage = downloads.storage
  const { totalPages, page } = useCollectorPagination({ collectorPage: 'downloads', state: filter, total: counts ? downloadTotal(counts, filter) : 0, pageSize: downloadPageSize, page: params.page, loaded: !!counts })
  const tabs: TableTab[] = filters.map((item) => ({ ...item, count: counts && downloadTotal(counts, item.state) }))

  const storageLabel = storage?.paused ? 'Storage: downloads paused'
    : storage ? `Storage: ${storage.available_bytes === null ? 'free space unknown' : `${formatBytes(storage.available_bytes)} free`}` : ''

  useHeaderAside(storageLabel || null)

  return (
    <Flex direction="column" gap="4" asChild><section aria-label="Download queue">
      {storage?.paused && <Text as="p" size="2" color="green" role="status">
        {storage.reason === 'space_check_failed'
          ? 'Downloads paused: the collector cannot check available storage.'
          : 'Downloads paused: collector storage is low.'}
        {' '}{storage.available_bytes === null ? 'Free space unknown.' : `${formatBytes(storage.available_bytes)} free.`}
        {' '}Downloads resume automatically at {formatBytes(storage.resume_at_bytes)} free. Completed downloads can still be added to a library.
      </Text>}
      <Grid columns={{ initial: '1', md: '2' }} gap="4" align="start">
        <Card><Flex direction="column" gap="2" minWidth="0">
          <Heading as="h2" size="3" id="add-download-title">Add a download</Heading>
          <form aria-labelledby="add-download-title" onSubmit={async (event) => {
            event.preventDefault()
            if (await downloads.submit(url)) {
              setURL('')
              window.location.hash = collectorLink('downloads')
            }
          }}>
            <Flex align="center" wrap="wrap" gap="2">
              <Flex flexGrow="1" minWidth="160px">
                <TextField.Root size="2" type="url" required aria-label="Panda gallery URL" style={{ width: '100%' }} value={url} onChange={(event) => setURL(event.target.value)} disabled={downloads.disabled} placeholder="Paste a Panda gallery URL" autoComplete="off" />
              </Flex>
              <Button size="2" variant="solid" type="submit" disabled={downloads.disabled || !url.trim()}>{downloads.pending ? 'Requesting…' : 'Add download'}</Button>
            </Flex>
          </form>
          <Flex align="center" gap="2" wrap="wrap">
            <Text size="1" color="gray">Submitting a gallery again reuses its existing download.</Text>
            <Popover.Root>
              <Popover.Trigger><Button size="1" variant="ghost" color="gray">Download details</Button></Popover.Trigger>
              <Popover.Content maxWidth="360px">
                <Flex direction="column" gap="3">
                  <Heading as="h3" size="3">Download original archives</Heading>
                  <Text as="p" size="2">Original archives are retained on the collector. Failed or cancelled jobs can be retried below.</Text>
                  <Text as="p" size="2">Save ZIP copies an archive to your device and keeps the collector copy. Add to library transfers it into Tana.</Text>
                </Flex>
              </Popover.Content>
            </Popover.Root>
          </Flex>
        </Flex></Card>
        <LibraryDeliveries deliveries={deliveries} libraryID={libraryID} onLibraryChange={setLibraryID} canStart={canDeliver} completedCount={counts?.completed ?? 0} />
      </Grid>
      {downloads.notice && <Text as="p" size="2" color="green" role="status">{downloads.notice}</Text>}
      {downloads.requestError && <Text as="p" size="2" color="red" role="alert">{downloads.requestError}</Text>}
      <ResourceStatus name="Downloads" plural loaded={!!counts} available={available} error={downloads.error} stale={downloads.stale} />
      {counts && <TableCard caption={`${filterLabel} downloads`} tabsLabel="Filter downloads" collectorPage="downloads" tabs={tabs} activeState={filter} aside="Newest first"
        columns={[undefined, 200, 100, 180, 280]} minWidth={920}
        footnote={filter === 'queued' ? 'Pending downloads are queued or waiting for storage space, an automatic retry, or Panda cooldown.' : undefined}
        empty={downloads.loaded && downloads.jobs.length === 0 && <Text as="p" size="2" role="status">{filter
          ? `No ${filterLabel.toLowerCase()} downloads.`
          : 'No downloads yet. Paste a Panda gallery URL to get started.'}</Text>}>
        <Table.Header><Table.Row>
          <Table.ColumnHeaderCell scope="col">Gallery</Table.ColumnHeaderCell>
          <Table.ColumnHeaderCell scope="col">State</Table.ColumnHeaderCell>
          <Table.ColumnHeaderCell scope="col">Size</Table.ColumnHeaderCell>
          <Table.ColumnHeaderCell scope="col">Updated</Table.ColumnHeaderCell>
          <Table.ColumnHeaderCell scope="col" justify="end"><VisuallyHidden>Actions</VisuallyHidden></Table.ColumnHeaderCell>
        </Table.Row></Table.Header>
        <Table.Body>{downloads.jobs.map((job) => {
          const deletable = ['completed', 'failed', 'cancelled', 'deleting'].includes(job.state)
          const owned = deliveryOwnsArchive(job.gallery_id)
          return <Table.Row key={job.gallery_id}>
            <Table.RowHeaderCell scope="row">Gallery {job.gallery_id}</Table.RowHeaderCell>
            <Table.Cell><StatusBadge tone={tones[job.state]} detail={jobDetails(job)}>{downloadState(job)}</StatusBadge></Table.Cell>
            <Table.Cell>{job.state === 'completed' ? formatBytes(job.size_bytes) : job.expected_size_bytes ? `~ ${formatBytes(job.expected_size_bytes)}` : '—'}</Table.Cell>
            <Table.Cell>{formatDate(job.updated_at)}<Text as="div" size="1" color="gray" mt="1">Added {formatDate(job.created_at)}</Text></Table.Cell>
            <Table.Cell><RowActions slot1Width={120} slot2Width={96} deleteSlot
              slot1={job.state === 'completed'
                ? <Button size="2" variant="solid" style={{ width: '100%' }} disabled={!canDeliver || owned} onClick={() => void deliveries.start({ library_id: libraryID, gallery_id: job.gallery_id })}>Add to library</Button>
                : job.state === 'queued' || job.state === 'running'
                  ? <Button size="2" variant="soft" color="gray" style={{ width: '100%' }} disabled={actionsDisabled} onClick={() => void downloads.change(job.gallery_id, 'cancel')}>Cancel</Button>
                  : job.state === 'failed' || job.state === 'cancelled'
                    ? <Button size="2" variant="soft" color="gray" style={{ width: '100%' }} disabled={actionsDisabled} onClick={() => void downloads.change(job.gallery_id, 'retry')}>Retry</Button>
                    : undefined}
              slot2={job.state === 'completed' ? (actionsDisabled
                ? <Button size="2" variant="soft" color="gray" style={{ width: '100%' }} disabled>Save ZIP</Button>
                : <Button size="2" variant="soft" color="gray" style={{ width: '100%' }} asChild><a href={downloadFileURL(job.gallery_id)} download>Save ZIP</a></Button>) : undefined}
              onDelete={deletable ? () => { setLastDeleting(job.gallery_id); setDeleting(job.gallery_id) } : undefined} deleteLabel={`Delete download ${job.gallery_id}`} deleteDisabled={actionsDisabled || owned} /></Table.Cell>
          </Table.Row>
        })}</Table.Body>
      </TableCard>}
      {counts && <CollectorPagination collectorPage="downloads" state={filter} page={page} totalPages={totalPages} label="Download pages" />}
      <AlertDialog.Root open={deleting !== null} onOpenChange={(open) => { if (!open) setDeleting(null) }}>
        <AlertDialog.Content maxWidth="420px">
          <AlertDialog.Title>Delete download {deleting ?? lastDeleting}?</AlertDialog.Title>
          <AlertDialog.Description size="2">Delete this download and any archive retained on the collector?</AlertDialog.Description>
          {deleting !== null && downloads.requestError && <Text as="p" size="2" color="red" mt="3" role="alert">{downloads.requestError}</Text>}
          <Flex gap="3" mt="4" justify="end">
            <AlertDialog.Cancel><Button size="2" variant="soft" color="gray" disabled={downloads.pending}>Keep download</Button></AlertDialog.Cancel>
            <Button size="2" variant="solid" color="red" disabled={actionsDisabled || deleting === null || deliveryOwnsArchive(deleting)}
              onClick={async () => { if (deleting !== null && await downloads.change(deleting, 'delete')) setDeleting(null) }}>Delete download</Button>
          </Flex>
        </AlertDialog.Content>
      </AlertDialog.Root>
    </section></Flex>
  )
}
