import { Badge, Button, Callout, Card, Flex, Heading, Link, Text } from '@radix-ui/themes'
import { usePandaCollection } from '../panda/usePandaCollection'

export default function FeedCapture({ available, onCaptured }: { available: boolean, onCaptured: () => void }) {
  const collection = usePandaCollection()
  const feed = collection.feed
  const capturing = collection.pending || feed?.capture_active

  return <Card>
    <Flex direction="column" gap="3" aria-label="Panda collection status">
      <Flex align="center" justify="between" wrap="wrap" gap="3">
        <Heading as="h2" size="4">Capture latest feed</Heading>
        <Button disabled={!available || capturing} onClick={async () => { await collection.capture(); onCaptured() }}>
          {capturing ? 'Capturing feed…' : 'Refresh feed'}
        </Button>
      </Flex>
      <Text size="2" as="p">{feed
        ? feed.capture_active ? 'Feed capture in progress.' : feed.last_captured_at
          ? <>Last feed captured <time dateTime={feed.last_captured_at}>{new Date(feed.last_captured_at).toLocaleString()}</time>.</>
          : 'No feed captured yet.'
        : 'Checking feed status…'}</Text>
      <Flex gap="2" wrap="wrap">
        {feed && <Badge color={feed.processing_pending ? 'amber' : 'green'}>
          {feed.processing_pending > 0 ? `${feed.processing_pending} feeds awaiting processing.` : 'Feed processing complete.'}
        </Badge>}
        {feed && <Badge color={feed.possible_gaps ? 'amber' : 'gray'}>
          {feed.possible_gaps ? `${feed.possible_gaps} possible feed gaps.` : feed.continuity === 'overlap' ? 'Latest feed overlaps earlier captures.' : 'Feed continuity unknown.'}
        </Badge>}
      </Flex>
      <Text size="1" color="gray">Captures discover gallery references for metadata collection. Browse collected metadata in <Link href="#/panda">Panda</Link>.</Text>
      {feed?.last_capture_error && <Text size="2" color="red">Feed capture: {feed.last_capture_error}</Text>}
      {feed?.processing_error && <Text size="2" color="red">Feed processing: {feed.processing_error}</Text>}
      {(collection.error || collection.requestError) && <Callout.Root color="red" role="alert">
        <Callout.Text>{collection.requestError || collection.error}</Callout.Text>
        <Button variant="soft" color="red" onClick={collection.retry}>Retry feed status</Button>
      </Callout.Root>}
    </Flex>
  </Card>
}
