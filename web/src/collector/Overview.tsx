import type { ReactNode } from 'react'
import { Card, Flex, Grid, Heading, Table, Text } from '@radix-ui/themes'
import type { CollectorPageProps } from './pageProps'
import type { InventoryStatus } from './api'
import { getInventoryStatus, getMetadataStatus } from './api'
import { attentionItems } from './attention'
import { useCollectorResource } from './useCollectorResource'
import ResourceStatus from './ResourceStatus'
import Stat from './Stat'
import type { Tone } from './StatusBadge'
import AttentionList from './AttentionList'
import MetadataCollection from './MetadataCollection'
import FeedMetadataDelay from './FeedMetadataDelay'
import { formatBytes } from './formatBytes'
import './Overview.css'
import { formatDate } from './formatDate'

function ServiceCard({ title, value, tone, detail, children }: { title: string, value: string, tone?: Tone, detail?: string, children?: ReactNode }) {
  return <Card size="2" className={tone === 'amber' ? 'collector-card-amber' : undefined}><Flex direction="column" gap="2">
    <Text size="1" color="gray">{title}</Text>
    <Flex align="center" gap="2">
      {tone && <span className="overview-dot" aria-hidden style={{ color: `var(--${tone}-9)` }} />}
      <Text size="4" weight="medium" color={tone === 'amber' ? 'amber' : undefined}>{value}</Text>
    </Flex>
    {detail && <Text size="1" color="gray">{detail}</Text>}
    {children}
  </Flex></Card>
}

function cooldownCard(title: string, label: string, unknown: boolean, until?: string) {
  if (unknown) return <ServiceCard title={title} value="Unknown" tone="gray" detail={label} />
  return until
    ? <ServiceCard title={title} value="Cooldown" tone="amber" detail={`Until ${formatDate(until)}`} />
    : <ServiceCard title={title} value="No cooldown" tone="green" detail={label} />
}

function InventoryCard({ counts }: { counts: InventoryStatus }) {
  const total = counts.gallery_references
  const share = (value: number) => total > 0 ? (value / total) * 100 : 0
  const parts = [
    { label: 'available', value: counts.metadata_available, color: 'var(--green-9)' },
    { label: 'pending', value: counts.metadata_pending, color: 'var(--gray-8)' },
    { label: 'failed', value: counts.metadata_failed, color: 'var(--red-9)' },
  ]
  const stats = [
    { label: 'Gallery references', value: counts.gallery_references }, { label: 'Metadata available', value: counts.metadata_available, color: 'green' as const },
    { label: 'Metadata pending', value: counts.metadata_pending }, { label: 'Metadata failed', value: counts.metadata_failed, color: 'red' as const },
  ]
  return <>
    <Grid columns={{ initial: '2', sm: '4' }} gap="4" asChild><dl style={{ margin: 0 }}>
      {stats.map((stat) => <Stat key={stat.label} {...stat} />)}
    </dl></Grid>
    <div role="img" className="overview-bar" aria-label={parts.filter((part) => part.value > 0).map((part) => `${share(part.value).toFixed(1)}% ${part.label}`).join(', ')}>
      {parts.filter((part) => part.value > 0).map((part) => <span key={part.label} style={{ width: `${share(part.value)}%`, background: part.color }} />)}
    </div>
    <Text as="p" size="1" color="gray">Pending and failed counts cover references without retained metadata.</Text>
  </>
}

export default function Overview({ available, refreshKey, interval, summary, summaryStale, refreshSummary }: CollectorPageProps) {
  const inventory = useCollectorResource(getInventoryStatus, available, refreshKey, interval)
  const metadata = useCollectorResource(getMetadataStatus, available, refreshKey, interval)
  const counts = inventory.data
  const diagnostics = metadata.data
  const paused = diagnostics?.main_background_paused ?? summary?.metadata.main_background_paused
  const storage = summary?.downloads.storage
  const metadataActionable = available && !metadata.stale

  return <>
    <AttentionList items={attentionItems(summary)} available={available && !summaryStale} onReset={() => { metadata.refresh(); refreshSummary?.() }} />
    <Flex direction="column" gap="2">
      <ResourceStatus name="Metadata collection status" loaded={!!diagnostics} available={available} {...metadata} />
      <ResourceStatus name="Summary data" loaded={!!summary} available={available} error="" stale={!!summaryStale} />
      <Grid columns={{ initial: '1', xs: '2', md: '4' }} gap="3" asChild><section aria-label="Service status">
        <ServiceCard title="Metadata collection" value={paused === undefined ? 'Unknown' : paused ? 'Paused' : 'Running'}
          tone={paused === undefined ? 'gray' : paused ? 'amber' : 'green'}>
          {diagnostics && <MetadataCollection paused={diagnostics.main_background_paused} available={metadataActionable}
            onChanged={(paused) => metadata.update({ ...diagnostics, main_background_paused: paused })} />}
        </ServiceCard>
        {cooldownCard('Main Panda requests', 'Unauthenticated', !summary, summary?.metadata.upstream_cooldown_until)}
        {cooldownCard('Authenticated requests', 'Favorites and archives', !summary, summary?.favorites.authenticated_cooldown_until)}
        {storage?.available_bytes != null
          ? <ServiceCard title="Download storage" value={`${formatBytes(storage.available_bytes)} free`} tone={storage.paused ? 'amber' : undefined}
            detail={`Pauses below ${formatBytes(storage.pause_below_bytes)}`} />
          : <ServiceCard title="Download storage" value="Unknown" tone="gray" />}
      </section></Grid>
    </Flex>
    <Card size="3"><Flex direction="column" gap="4" asChild><section aria-labelledby="inventory-title">
      <Flex direction="column" gap="1">
        <Heading as="h2" size="4" id="inventory-title">Collector inventory</Heading>
        {counts && <Text size="1" color="gray">Explicit fetch requests: {counts.fetches_pending.toLocaleString()} pending · {counts.fetches_failed.toLocaleString()} failed</Text>}
      </Flex>
      <ResourceStatus name="Inventory statistics" plural loaded={!!counts} available={available} {...inventory} />
      {counts && <InventoryCard counts={counts} />}
    </section></Flex></Card>
    <Grid columns={{ initial: '1', md: '3' }} gap="4" align="start">
      <Card size="3" className="overview-errors"><Flex direction="column" gap="3" asChild><section aria-labelledby="errors-title">
        <Flex justify="between" align="baseline" wrap="wrap" gap="2">
          <Heading as="h2" size="4" id="errors-title">Recent metadata errors</Heading>
          {diagnostics?.metadata_retry_at && <Text size="1" color="gray">Next retry after {formatDate(diagnostics.metadata_retry_at)}</Text>}
        </Flex>
        {diagnostics?.metadata_last_error && <Text as="p" size="2" color="red">Metadata collection: {diagnostics.metadata_last_error}</Text>}
        {diagnostics && diagnostics.metadata_errors.length === 0 && <Text as="p" size="2" color="gray" role="status">No recent metadata errors.</Text>}
        {diagnostics && diagnostics.metadata_errors.length > 0 && <div style={{ overflowX: 'auto' }}>
          <Table.Root size="2" layout="fixed" style={{ minWidth: 520 }}>
            <colgroup><col style={{ width: 120 }} /><col /><col style={{ width: 180 }} /></colgroup>
            <Table.Header><Table.Row>
              <Table.ColumnHeaderCell>Gallery</Table.ColumnHeaderCell><Table.ColumnHeaderCell>Error</Table.ColumnHeaderCell>
              <Table.ColumnHeaderCell justify="end">When</Table.ColumnHeaderCell>
            </Table.Row></Table.Header>
            <Table.Body>{diagnostics.metadata_errors.map((item) => <Table.Row key={item.gallery_id}>
              <Table.RowHeaderCell>{item.gallery_id}</Table.RowHeaderCell>
              <Table.Cell>{item.error}</Table.Cell>
              <Table.Cell justify="end">{item.at ? formatDate(item.at) : '—'}</Table.Cell>
            </Table.Row>)}</Table.Body>
          </Table.Root>
        </div>}
      </section></Flex></Card>
      <Card size="3" className="overview-settings"><Flex direction="column" gap="3" asChild><section aria-labelledby="settings-title">
        <Heading as="h2" size="4" id="settings-title">Collection settings</Heading>
        {diagnostics && <FeedMetadataDelay hours={diagnostics.feed_metadata_delay_hours} available={metadataActionable}
          onSaved={(hours) => metadata.update({ ...diagnostics, feed_metadata_delay_hours: hours })} />}
      </section></Flex></Card>
    </Grid>
  </>
}
