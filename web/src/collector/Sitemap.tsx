import { Button, Flex, Grid, Heading, Progress, Select, Text } from '@radix-ui/themes'
import { useEffect, useRef, useState } from 'react'
import { updateSitemap } from './api'
import type { SitemapAction, SitemapStatus } from './api'

function date(value: string) { return new Date(value).toLocaleString() }

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

export default function Sitemap({ status, available, onChanged }: {
  status: SitemapStatus
  available: boolean
  onChanged: (status: SitemapStatus) => void
}) {
  const [force, setForce] = useState(false)
  const [pending, setPending] = useState<SitemapAction | null>(null)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const mutation = useRef<AbortController | null>(null)
  useEffect(() => () => mutation.current?.abort(), [])
  const running = status.state === 'running'
  const disabled = !available || pending !== null
  const processed = status.children_completed + status.children_skipped + status.children_failed

  async function request(action: SitemapAction) {
    if (mutation.current || !available) return
    const controller = new AbortController()
    mutation.current = controller
    setPending(action)
    setError('')
    setNotice('')
    try {
      const result = await updateSitemap(action, force, controller.signal)
      if (!controller.signal.aborted) {
        onChanged(result)
        setNotice(action === 'cancel' ? 'Sitemap cancellation requested. Imported references and metadata work are retained.'
          : action === 'retry' ? 'Sitemap retry requested.' : 'Sitemap collection requested.')
      }
    } catch (error) {
      if (!controller.signal.aborted) setError((error as Error).message)
    } finally {
      mutation.current = null
      if (!controller.signal.aborted) setPending(null)
    }
  }

  return <Flex direction="column" gap="4" asChild><section aria-labelledby="sitemap-title">
    <Flex align="center" justify="between" wrap="wrap" gap="3"><Heading as="h2" size="4" id="sitemap-title">Collect gallery references</Heading><span>{stateLabel(status)}{status.force && status.state !== 'idle' ? ' · Force refresh' : ''}</span></Flex>
    <Text as="p" size="2">Collect gallery references for automatic metadata fetching. Fresh discoveries take priority over sitemap backfill.</Text>
    <Flex align="end" wrap="wrap" gap="3" asChild><form onSubmit={(event) => { event.preventDefault(); if (!disabled && !running) void request('start') }}>
      <Flex direction="column" gap="2" flexGrow="1" asChild><label>Collection mode
        <Select.Root value={force ? 'force' : 'conditional'} onValueChange={(value) => setForce(value === 'force')} disabled={disabled || running}><Select.Trigger aria-label="Collection mode" /><Select.Content>
          <Select.Item value="conditional">Skip unchanged sitemaps</Select.Item>
          <Select.Item value="force">Force refresh all sitemaps</Select.Item>
        </Select.Content></Select.Root>
      </label></Flex>
      <Button size="2" variant="solid" type="submit" disabled={disabled || running}>{pending === 'start' ? 'Requesting…' : 'Collect sitemap'}</Button>
      {running && <Button size="2" variant="soft" type="button" disabled={disabled} onClick={() => void request('cancel')} color="gray">{pending === 'cancel' ? 'Cancelling…' : 'Cancel sitemap collection'}</Button>}
      {(status.state === 'incomplete' || status.state === 'cancelled') && <Button size="2" variant="soft" type="button" disabled={disabled} onClick={() => void request('retry')} color="gray">{pending === 'retry' ? 'Requesting…' : status.state === 'cancelled' ? 'Resume sitemap collection' : 'Retry sitemap collection'}</Button>}
    </form></Flex>
    {status.state !== 'idle' && <>
      <Text as="p" size="2">{processed.toLocaleString()} of {status.children_total.toLocaleString()} child sitemaps processed{running && status.children_total === 0 ? ' · Reading index' : ''}</Text>
      <Progress aria-label="Sitemap collection progress" value={status.children_total ? processed : running ? undefined : 0} max={status.children_total || 1} />
      <Grid columns={{ initial: '2', sm: '4' }} gap="3" my="2" asChild><dl>
        <div><Text size="1" color="gray" asChild><dt>Child sitemaps imported</dt></Text><Text size="5" weight="medium" asChild><dd style={{ margin: 0 }}>{status.children_completed.toLocaleString()}</dd></Text></div>
        <div><Text size="1" color="gray" asChild><dt>Unchanged sitemaps skipped</dt></Text><Text size="5" weight="medium" asChild><dd style={{ margin: 0 }}>{status.children_skipped.toLocaleString()}</dd></Text></div>
        <div><Text size="1" color="gray" asChild><dt>Child sitemaps failed</dt></Text><Text size="5" weight="medium" asChild><dd style={{ margin: 0 }}>{status.children_failed.toLocaleString()}</dd></Text></div>
        <div><Text size="1" color="gray" asChild><dt>Invalid gallery URLs skipped</dt></Text><Text size="5" weight="medium" asChild><dd style={{ margin: 0 }}>{status.invalid_locations.toLocaleString()}</dd></Text></div>
        <div><Text size="1" color="gray" asChild><dt>Gallery references found</dt></Text><Text size="5" weight="medium" asChild><dd style={{ margin: 0 }}>{status.references_found.toLocaleString()}</dd></Text></div>
        <div><Text size="1" color="gray" asChild><dt>Gallery references imported</dt></Text><Text size="5" weight="medium" asChild><dd style={{ margin: 0 }}>{status.references_imported.toLocaleString()}</dd></Text></div>
      </dl></Grid>
      {status.started_at && <Text as="p" size="1" color="gray">Started {date(status.started_at)}{status.finished_at ? ` · Finished ${date(status.finished_at)}` : ''}</Text>}
      {running && status.retry_at && <Text as="p" size="1" color="gray">Continue after {date(status.retry_at)}.</Text>}
    </>}
    <Text as="p" size="1" color="gray">Sitemap completion means reference import is complete. Metadata fetching continues independently; its backlog appears in Collector inventory.</Text>
    {status.last_error && <Text as="p" size="2" color="red">Sitemap collection: {errorLabel(status.last_error)}</Text>}
    {notice && <Text as="p" size="2" color="green" role="status">{notice}</Text>}
    {error && <Text as="p" size="2" color="red" role="alert">{error}</Text>}
  </section></Flex>
}
