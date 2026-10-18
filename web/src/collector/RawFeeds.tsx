import { Button, Flex, Table, Text, VisuallyHidden } from '@radix-ui/themes'
import { useCallback, useState } from 'react'
import type { CollectorPageProps } from './pageProps'
import { useCollectorListParams } from '../navigation'
import { feedCaptureFileURL, getFeedCaptures } from './api'
import type { FeedCapture as FeedCaptureRecord } from './api'
import CollectorPagination from './CollectorPagination'
import { useCollectorPagination } from './useCollectorPagination'
import ResourceStatus from './ResourceStatus'
import RowActions from './RowActions'
import StatusBadge from './StatusBadge'
import type { Tone } from './StatusBadge'
import TableCard from './TableCard'
import { useCollectorResource } from './useCollectorResource'
import FeedCapture from './FeedCapture'
import { formatBytes } from './formatBytes'
import { formatDate } from './formatDate'

const pageSize = 25
const states: Record<FeedCaptureRecord['state'], string> = { pending: 'Pending', processed: 'Processed', failed: 'Failed' }
const tones: Record<FeedCaptureRecord['state'], Tone> = { pending: 'amber', processed: 'green', failed: 'red' }
const tabs = [{ state: '', label: 'All' }, { state: 'failed', label: 'Failed', color: 'red' as const }]

export default function RawFeeds({ available, refreshKey, interval }: CollectorPageProps) {
  const { page, state } = useCollectorListParams()
  const failedOnly = state === 'failed'
  const [captureRevision, setCaptureRevision] = useState(0)

  return <Flex direction="column" gap="4" asChild><section aria-labelledby="feed-captures-title">
    <FeedCapture available={available} refreshKey={refreshKey} interval={interval} onCaptured={() => setCaptureRevision((value) => value + 1)} />
    <CaptureList key={failedOnly ? 'failed' : 'all'} available={available} refreshKey={refreshKey + captureRevision} interval={interval} failedOnly={failedOnly} page={page} />
  </section></Flex>
}

// Not keyed on the page: the previous response stays until the next arrives, so the pagination keeps its total.
function CaptureList({ available, refreshKey, interval, failedOnly, page }: {
  available: boolean, refreshKey: number, interval: number, failedOnly: boolean, page: number,
}) {
  const state = failedOnly ? 'failed' : ''
  // The total is unknown before the first response, so the request trusts the URL; a page past the end is corrected afterwards.
  const load = useCallback((signal: AbortSignal) => getFeedCaptures(failedOnly, pageSize, (page - 1) * pageSize, signal), [failedOnly, page])
  const resource = useCollectorResource(load, available, refreshKey, interval)
  const { data } = resource
  const { totalPages, page: currentPage } = useCollectorPagination({ collectorPage: 'feeds', state, total: data?.total ?? 0, pageSize, page, loaded: !!data })

  return <>
    <ResourceStatus name="Feed captures" plural loaded={!!data} available={available} {...resource} />
    {data && <TableCard title="Feed captures" titleId="feed-captures-title" caption={`${failedOnly ? 'Failed captures' : 'All captures'} · newest first`}
      empty={data.captures.length === 0 && <Text as="p" size="2" role="status">{failedOnly ? 'No failed feed captures.' : 'No feed captures.'}</Text>}
      collectorPage="feeds" tabs={tabs} activeState={state} aside="Newest first" columns={[120, 160, undefined, 120, 184]}
      footnote="Pending and failed feeds remain available to download, including malformed XML. Processed feeds are cleaned up automatically. Capture IDs match collector logs.">
      <Table.Header><Table.Row>
        <Table.ColumnHeaderCell scope="col">Capture</Table.ColumnHeaderCell>
        <Table.ColumnHeaderCell scope="col">Captured</Table.ColumnHeaderCell>
        <Table.ColumnHeaderCell scope="col">Processing</Table.ColumnHeaderCell>
        <Table.ColumnHeaderCell scope="col" justify="end">Size</Table.ColumnHeaderCell>
        <Table.ColumnHeaderCell scope="col" justify="end"><VisuallyHidden>Actions</VisuallyHidden></Table.ColumnHeaderCell>
      </Table.Row></Table.Header>
      <Table.Body>{data.captures.map((capture) => <Table.Row key={capture.id}>
        <Table.RowHeaderCell scope="row">{capture.id}</Table.RowHeaderCell>
        <Table.Cell>{formatDate(capture.captured_at)}</Table.Cell>
        <Table.Cell><StatusBadge tone={tones[capture.state]} detail={capture.error}>{states[capture.state]}</StatusBadge></Table.Cell>
        <Table.Cell justify="end">{formatBytes(capture.size_bytes)}</Table.Cell>
        <Table.Cell><RowActions slot1Width={160} slot1={available
          ? <Button size="2" variant="soft" asChild color="gray" style={{ width: '100%' }}><a href={feedCaptureFileURL(capture.id)} download aria-label={`Download raw feed ${capture.id}`}>Download raw feed</a></Button>
          : <Button size="2" variant="soft" disabled color="gray" style={{ width: '100%' }}>Download raw feed</Button>} /></Table.Cell>
      </Table.Row>)}</Table.Body>
    </TableCard>}
    <CollectorPagination collectorPage="feeds" state={state} page={currentPage} totalPages={totalPages} label="Feed capture pages" />
  </>
}
