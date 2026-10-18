import { Badge, Button, Callout, Card, Flex, Heading, Text } from '@radix-ui/themes'
import { getFeedStatus, refreshFeed } from '../panda/api'
import ResourceStatus from './ResourceStatus'
import { useCollectorResource } from './useCollectorResource'
import { useMutation } from './useMutation'
import { formatDate } from './formatDate'

export default function FeedCapture({ available, refreshKey, interval, onCaptured }: {
  available: boolean, refreshKey: number, interval: number, onCaptured: () => void,
}) {
  const resource = useCollectorResource(getFeedStatus, available, refreshKey, interval)
  const feed = resource.data
  const mutation = useMutation()
  const capturing = mutation.pending || feed?.capture_active

  function capture() {
    void mutation.run(async (signal) => resource.update(await refreshFeed(signal)), { onSuccess: onCaptured })
  }

  return <Card>
    <Flex direction="column" gap="3" asChild><section aria-labelledby="latest-feed-title">
      <Flex align="center" justify="between" wrap="wrap" gap="3">
        <Flex direction="column" gap="1">
          <Heading as="h2" size="4" id="latest-feed-title">Latest feed</Heading>
          <Text size="2" color="gray">{feed
            ? feed.capture_active ? 'Feed capture in progress.' : feed.last_captured_at
              ? <>Last captured <time dateTime={feed.last_captured_at}>{formatDate(feed.last_captured_at)}</time></>
              : 'No feed captured yet.'
            : 'Checking feed status…'}</Text>
        </Flex>
        <Button disabled={!available || resource.stale || capturing} onClick={capture}>
          {capturing ? 'Capturing feed…' : 'Refresh feed'}
        </Button>
      </Flex>
      <ResourceStatus name="Feed status" loaded={!!feed} available={available} {...resource} />
      {feed && <Flex gap="2" wrap="wrap">
        <Badge size="1" color={feed.processing_pending ? 'amber' : 'green'}>
          {feed.processing_pending > 0 ? `${feed.processing_pending} ${feed.processing_pending === 1 ? 'feed' : 'feeds'} awaiting processing` : 'Feed processing complete'}
        </Badge>
        <Badge size="1" color={feed.possible_gaps ? 'amber' : 'gray'}>
          {feed.possible_gaps ? `${feed.possible_gaps} possible feed ${feed.possible_gaps === 1 ? 'gap' : 'gaps'}` : feed.continuity === 'overlap' ? 'Latest feed overlaps earlier captures' : 'Feed continuity unknown'}
        </Badge>
      </Flex>}
      {feed?.last_capture_error && <Text size="2" color="red">Feed capture: {feed.last_capture_error}</Text>}
      {feed?.processing_error && <Callout.Root color="red" role="status" size="1">
        <Callout.Text><Text weight="bold" color="red">Feed processing stopped</Text> · {feed.processing_error}</Callout.Text>
      </Callout.Root>}
      {mutation.error && <Callout.Root color="red" role="alert">
        <Callout.Text>{mutation.error}</Callout.Text>
      </Callout.Root>}
    </section></Flex>
  </Card>
}
