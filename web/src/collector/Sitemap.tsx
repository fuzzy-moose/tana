import { Badge, Box, Button, Callout, Card, Flex, Grid, Heading, Link, Progress, Select, Text } from '@radix-ui/themes'
import { useState } from 'react'
import { getSitemapStatus, updateSitemap } from './api'
import type { SitemapAction, SitemapStatus } from './api'
import type { CollectorPageProps } from './pageProps'
import ResourceStatus from './ResourceStatus'
import Stat from './Stat'
import type { Tone } from './StatusBadge'
import { useCollectorResource } from './useCollectorResource'
import { useMutation } from './useMutation'
import './Sitemap.css'
import { formatDate } from './formatDate'

function errorLabel(error: string) {
  if (error === 'child_sitemaps_failed') return 'Some child sitemaps could not be collected. Retry to continue.'
  if (error === 'invalid_sitemap') return 'A sitemap could not be parsed.'
  if (error === 'request_failed') return 'A sitemap request failed.'
  if (error === 'panda_banned') return 'Panda temporarily restricted access.'
  if (/^upstream_http_\d+$/.test(error)) return `Panda returned HTTP ${error.slice('upstream_http_'.length)}.`
  return error
}

function stateLabel(status: SitemapStatus) {
  if (status.state === 'running') return status.retry_at && Date.parse(status.retry_at)> Date.now() ? 'Waiting to continue' : 'Collecting'
  if (status.state === 'completed') return 'Completed'
  if (status.state === 'incomplete') return 'Incomplete'
  if (status.state === 'cancelled') return 'Cancelled'
  return 'Idle'
}

const stateTones: Record<SitemapStatus['state'], Tone> = { idle: 'gray', running: 'blue', completed: 'green', incomplete: 'amber', cancelled: 'gray' }

export default function Sitemap({ available, refreshKey, interval }: CollectorPageProps) {
  const sitemap = useCollectorResource(getSitemapStatus, available, refreshKey, interval)
  return <Flex direction="column" gap="4">
    <ResourceStatus name="Sitemap statistics" plural loaded={!!sitemap.data} available={available} {...sitemap} />
    {sitemap.data && <SitemapView status={sitemap.data} available={available && !sitemap.stale} onChanged={sitemap.update} />}
  </Flex>
}

function SitemapView({ status, available, onChanged }: {
  status: SitemapStatus
  available: boolean
  onChanged: (status: SitemapStatus) => void
}) {
  const [force, setForce] = useState(false)
  const [requested, setRequested] = useState<SitemapAction | null>(null)
  const { pending, error, notice, run } = useMutation()
  const running = status.state === 'running'
  const disabled = !available || pending
  const processed = status.children_completed + status.children_skipped + status.children_failed
  const percent = status.children_total ? Math.round(processed / status.children_total * 100) : 0

  function request(action: SitemapAction) {
    if (!available || pending) return
    setRequested(action)
    void run((signal) => updateSitemap(action, force, signal), {
      onSuccess: (result) => {
        onChanged(result)
        return action === 'cancel' ? 'Sitemap cancellation requested. Imported references and metadata work are retained.'
          : action === 'retry' ? 'Sitemap retry requested.' : 'Sitemap collection requested.'
      },
    })
  }

  return <Flex direction="column" gap="4">
    <Grid columns={{ initial: '1', md: '3fr 2fr' }} gap="3" align="stretch">
      <Card><Flex direction="column" gap="3" asChild><section aria-labelledby="sitemap-run-title">
        <Flex align="center" justify="between" wrap="wrap" gap="3">
          <Flex align="center" gap="2" wrap="wrap">
            <Heading as="h2" size="4" id="sitemap-run-title">Current collection</Heading>
            <Badge size="1" color={stateTones[status.state]}>{stateLabel(status)}</Badge>
            {status.force && status.state !== 'idle' && <Badge size="1" color="gray">Force refresh</Badge>}
          </Flex>
          {running && <Button variant="soft" color="gray" disabled={disabled} onClick={() => request('cancel')}>{pending && requested === 'cancel' ? 'Cancelling…' : 'Cancel collection'}</Button>}
        </Flex>
        {status.state === 'idle' ? <Text size="2" color="gray">No collection has run yet.</Text> : <>
          <Flex justify="between" align="baseline" wrap="wrap" gap="2">
            <Text>{processed.toLocaleString()} of {status.children_total.toLocaleString()} child sitemaps processed{running && status.children_total === 0 ? ' · Reading index' : ''}</Text>
            <Text color="gray">{percent}%</Text>
          </Flex>
          <Progress aria-label="Sitemap collection progress" value={status.children_total ? processed : running ? undefined : 0} max={status.children_total || 1} />
          {status.started_at && <Text size="1" color="gray">Started {formatDate(status.started_at)}{status.finished_at ? ` · Finished ${formatDate(status.finished_at)}` : ''}</Text>}
          {running && status.retry_at && <Text size="1" color="gray">Continue after {formatDate(status.retry_at)}.</Text>}
        </>}
      </section></Flex></Card>
      <Card><Flex direction="column" gap="2" asChild><form onSubmit={(event) => { event.preventDefault(); if (!disabled && !running) request('start') }}>
        <Text as="label" weight="medium" id="sitemap-mode-label">Start a collection</Text>
        <Flex gap="2" wrap="wrap">
          <Select.Root value={force ? 'force' : 'conditional'} onValueChange={(value) => setForce(value === 'force')} disabled={disabled || running}>
            <Box flexGrow="1" minWidth="0"><Select.Trigger aria-labelledby="sitemap-mode-label" style={{ width: '100%' }} /></Box><Select.Content>
              <Select.Item value="conditional">Skip unchanged sitemaps</Select.Item>
              <Select.Item value="force">Force refresh all sitemaps</Select.Item>
            </Select.Content>
          </Select.Root>
          <Button type="submit" disabled={disabled || running}>{pending && requested === 'start' ? 'Requesting…' : 'Collect sitemap'}</Button>
        </Flex>
        {(status.state === 'incomplete' || status.state === 'cancelled') && <Flex>
          <Button type="button" variant="soft" color="gray" disabled={disabled} onClick={() => request('retry')}>{pending && requested === 'retry' ? 'Requesting…' : status.state === 'cancelled' ? 'Resume collection' : 'Retry failed'}</Button>
        </Flex>}
        <Text size="1" color="gray">{running ? 'Available when the current collection finishes or is cancelled.' : 'Force refresh downloads every child sitemap again.'}</Text>
      </form></Flex></Card>
    </Grid>

    <Card><Flex direction="column" gap="4" asChild><section aria-labelledby="sitemap-results-title">
      <Heading as="h2" size="4" id="sitemap-results-title">Results so far</Heading>
      <Grid columns={{ initial: '1', md: '2' }} gap="6">
        <Flex direction="column" gap="3">
          <Heading as="h3" size="1" weight="medium" color="gray" className="sitemap-group-title">Child sitemaps</Heading>
          <Grid columns={{ initial: '2', xs: '3' }} gap="4" asChild><dl style={{ margin: 0 }}>
            <Stat label="Imported" value={status.children_completed} color="green" />
            <Stat label="Unchanged, skipped" value={status.children_skipped} />
            <Stat label="Failed" value={status.children_failed} color={status.children_failed ? 'red' : undefined} />
          </dl></Grid>
        </Flex>
        <Flex direction="column" gap="3" className="sitemap-results-divider">
          <Heading as="h3" size="1" weight="medium" color="gray" className="sitemap-group-title">Gallery references</Heading>
          <Grid columns={{ initial: '2', xs: '3' }} gap="4" asChild><dl style={{ margin: 0 }}>
            <Stat label="Found" value={status.references_found} />
            <Stat label="Imported" value={status.references_imported} color="green" />
            <Stat label="Invalid URLs skipped" value={status.invalid_locations} />
          </dl></Grid>
        </Flex>
      </Grid>
    </section></Flex></Card>

    {status.last_error && <Callout.Root color="red" role="status" size="1">
      <Callout.Text><Text weight="bold" color="red">Last error</Text> · {errorLabel(status.last_error)}</Callout.Text>
    </Callout.Root>}
    {notice && <Text as="p" size="2" color="green" role="status">{notice}</Text>}
    {error && <Text as="p" size="2" color="red" role="alert">{error}</Text>}
    <Text as="p" size="1" color="gray">Sitemap completion means reference import is complete. Metadata fetching continues independently; its backlog appears in <Link href="#/collector">Collector overview</Link>.</Text>
  </Flex>
}
