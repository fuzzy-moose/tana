import { Badge, Button, Flex, Heading, Select, Table, Text } from '@radix-ui/themes'
import { useCallback, useState } from 'react'
import { feedCaptureFileURL, getFeedCaptures } from './api'
import ResourceStatus from './ResourceStatus'
import { useCollectorResource } from './useCollectorResource'
import FeedCapture from './FeedCapture'

const pageSize = 25
const states = { pending: 'Pending', processed: 'Processed', failed: 'Failed' }

export default function RawFeeds({ available, refreshKey }: { available: boolean, refreshKey: number }) {
  const [failedOnly, setFailedOnly] = useState(false)
  const [offset, setOffset] = useState(0)
  const [captureRevision, setCaptureRevision] = useState(0)

  return <Flex direction="column" gap="4" asChild><section aria-labelledby="feed-captures-title">
    <FeedCapture available={available} onCaptured={() => setCaptureRevision((value) => value + 1)} />
    <Heading as="h2" size="4" id="feed-captures-title">Feed captures</Heading>
    <Text as="p" size="2">Pending and failed feeds remain available to download, including malformed XML. Successfully processed feeds are cleaned up automatically. Capture IDs match collector logs.</Text>
    <Flex align="end" wrap="wrap" gap="3">
      <Flex direction="column" gap="2" flexGrow="1" asChild><label>Show captures
        <Select.Root value={failedOnly ? 'failed' : 'all'} onValueChange={(value) => {
          setFailedOnly(value === 'failed')
          setOffset(0)
        }}><Select.Trigger aria-label="Show captures" /><Select.Content>
          <Select.Item value="all">All captures</Select.Item>
          <Select.Item value="failed">Failed only</Select.Item>
        </Select.Content></Select.Root>
      </label></Flex>
    </Flex>
    <CaptureList key={`${failedOnly}-${offset}`} available={available} refreshKey={refreshKey + captureRevision}
      failedOnly={failedOnly} offset={offset} setOffset={setOffset} />
  </section></Flex>
}

function CaptureList({ available, refreshKey, failedOnly, offset, setOffset }: {
  available: boolean, refreshKey: number, failedOnly: boolean, offset: number, setOffset: (offset: number) => void,
}) {
  const load = useCallback((signal: AbortSignal) => getFeedCaptures(failedOnly, pageSize, offset, signal), [failedOnly, offset])
  const resource = useCollectorResource(load, available, refreshKey, 30000)
  const { data } = resource

  return <>
    <ResourceStatus name="Feed captures" loaded={!!data} available={available} {...resource} />
    {data?.captures.length === 0 && <Text as="p" size="2" role="status">{failedOnly ? 'No failed feed captures.' : 'No feed captures.'}</Text>}
    {!!data?.captures.length && <div className="collector-table-scroll">
      <Table.Root size="1" variant="surface" className="collector-table collector-feeds">
        <Text size="1" color="gray" align="left" mb="2" asChild><caption>{failedOnly ? 'Failed captures' : 'All captures'} · newest first</caption></Text>
        <Table.Header><Table.Row><Table.ColumnHeaderCell scope="col">Capture</Table.ColumnHeaderCell><Table.ColumnHeaderCell scope="col">Captured</Table.ColumnHeaderCell><Table.ColumnHeaderCell scope="col">Processing</Table.ColumnHeaderCell><Table.ColumnHeaderCell scope="col">Size</Table.ColumnHeaderCell><Table.ColumnHeaderCell scope="col">Download</Table.ColumnHeaderCell></Table.Row></Table.Header>
        <Table.Body>{data.captures.map((capture) => <Table.Row key={capture.id}>
          <Table.RowHeaderCell scope="row">Capture {capture.id}</Table.RowHeaderCell>
          <Table.Cell>{new Date(capture.captured_at).toLocaleString()}</Table.Cell>
          <Table.Cell><Badge color={capture.state === 'failed' ? 'red' : capture.state === 'processed' ? 'green' : 'amber'}>{states[capture.state]}</Badge>
            {capture.error && <Text as="span" size="1" color="gray" style={{ display: 'block' }} mt="1">{capture.error}</Text>}
          </Table.Cell>
          <Table.Cell>{capture.size_bytes.toLocaleString()} B</Table.Cell>
          <Table.Cell>{available
            ? <Button size="2" variant="soft" asChild color="gray"><a href={feedCaptureFileURL(capture.id)} download aria-label={`Download raw feed ${capture.id}`}>Download raw feed</a></Button>
            : <Button size="2" variant="soft" disabled color="gray">Download raw feed</Button>}
          </Table.Cell>
        </Table.Row>)}</Table.Body>
      </Table.Root>
    </div>}
    {(offset > 0 || data?.has_more) && <Flex align="center" justify="end" wrap="wrap" gap="3" asChild><nav aria-label="Feed capture pages">
      <Button size="2" variant="soft" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - pageSize))} color="gray">Previous captures</Button>
      <span>Page {offset / pageSize + 1}</span>
      <Button size="2" variant="soft" disabled={!available || resource.checking || resource.stale || !data?.has_more} onClick={() => setOffset(offset + pageSize)} color="gray">Next captures</Button>
    </nav></Flex>}
  </>
}
