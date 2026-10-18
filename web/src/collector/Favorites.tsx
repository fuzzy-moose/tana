import { Button, Card, Checkbox, Flex, Grid, SegmentedControl, Select, Table, Text, VisuallyHidden } from '@radix-ui/themes'
import { useState } from 'react'
import { getFavoritesStatus, saveFavoriteDownloadSettings, syncFavorites } from './api'
import type { FavoriteCategory } from './api'
import MissingFavoriteDownloads from './MissingFavoriteDownloads'
import ResourceStatus from './ResourceStatus'
import RowActions from './RowActions'
import { detailLines } from './detailLines'
import StatusBadge from './StatusBadge'
import type { Tone } from './StatusBadge'
import TableCard from './TableCard'
import { useHeaderAside } from './headerAside'
import type { CollectorPageProps } from './pageProps'
import { useCollectorResource } from './useCollectorResource'
import { useMutation } from './useMutation'
import { formatDate } from './formatDate'

function categoryName(category: FavoriteCategory) { return category.name || `Category ${category.category}` }

function stateLabel(category: FavoriteCategory) {
  if (category.state === 'waiting_cooldown') return 'Waiting for cooldown'
  if (category.state === 'running') return category.full ? 'Running full re-sync' : 'Running sync'
  if (category.state === 'queued') return category.queued_full ? 'Full re-sync queued' : 'Sync queued'
  if (category.last_outcome === 'failed') return 'Failed'
  if (category.last_outcome === 'success') return 'Completed'
  return 'Idle'
}

function stateTone(category: FavoriteCategory): Tone {
  if (category.state === 'running') return 'blue'
  if (category.state === 'waiting_cooldown' || category.state === 'queued') return 'amber'
  if (category.last_outcome === 'failed') return 'red'
  if (category.last_outcome === 'success') return 'green'
  return 'gray'
}

function stateDetails(category: FavoriteCategory) {
  return detailLines([
    category.started_at && `${(category.entries_saved ?? 0).toLocaleString()} entries saved · ${(category.pages_saved ?? 0).toLocaleString()} pages`,
    category.started_at && (category.last_saved_at ? `Last save ${formatDate(category.last_saved_at)}` : 'Awaiting first page'),
    category.last_error && `${category.last_error}${category.last_error_at ? ` · ${formatDate(category.last_error_at)}` : ''}`,
    category.retry_at && `Retry after ${formatDate(category.retry_at)}`,
    category.last_outcome && category.finished_at && formatDate(category.finished_at),
    category.queued && category.state !== 'queued' && `${category.queued_full ? 'Full re-sync' : 'Sync'} also queued`,
  ])
}

// "5–9", "2, 5–9"
function categoryRanges(ids: number[]) {
  const ranges: string[] = []
  for (let i = 0; i < ids.length; i++) {
    let j = i
    while (ids[j + 1] === ids[j] + 1) j++
    ranges.push(j > i ? `${ids[i]}–${ids[j]}` : String(ids[i]))
    i = j
  }
  return ranges.join(', ')
}

const baselineLabels = { not_started: 'Not started', ready: 'Ready' }

export default function Favorites({ available, refreshKey, interval }: CollectorPageProps) {
  const favorites = useCollectorResource(getFavoritesStatus, available, refreshKey, interval)
  const [category, setCategory] = useState('all')
  const [full, setFull] = useState(false)
  const [downloadCategory, setDownloadCategory] = useState<FavoriteCategory | null>(null)
  const [downloadPending, setDownloadPending] = useState(false)
  const [draft, setDraft] = useState<number[] | null>(null)
  const [showEmpty, setShowEmpty] = useState(false)
  const syncMutation = useMutation()
  const saveMutation = useMutation()
  const { pending: syncing } = syncMutation
  const { pending: saving } = saveMutation
  const status = favorites.data
  const disabled = !available || favorites.stale || syncing || downloadPending || !status

  function sync(category: string, full: boolean) {
    if (disabled) return
    const baseline = status?.downloads?.baseline_state
    void syncMutation.run((signal) => syncFavorites(category, full, signal), {
      onSuccess: () => baseline && baseline !== 'ready'
        ? 'Baseline sync requested for all ten categories.'
        : `${full ? 'Full re-sync' : 'Sync'} requested for ${category === 'all' ? 'all categories' : `category ${category}`}.`,
      onSettled: favorites.refresh,
    })
  }

  function save(selected: number[]) {
    if (disabled || !status?.downloads) return
    const { downloads } = status
    void saveMutation.run((signal) => saveFavoriteDownloadSettings(selected, signal), {
      onSuccess: (result) => {
        favorites.update({ ...status, downloads: { ...downloads, categories: result.categories } })
        setDraft(null)
        return 'Download categories saved.'
      },
    })
  }

  const selected = draft ?? status?.downloads?.categories ?? []
  // Based on the saved selection, so unchecking a box does not make its row vanish.
  const saved = status?.downloads?.categories ?? []
  const emptyCategories = status ? status.categories.filter((item) => item.favorites === 0 && item.state === 'idle' && !item.queued
    && !item.last_error && item.last_outcome !== 'failed' && !saved.includes(item.category)) : []
  const emptySynced = emptyCategories.map((item) => item.last_synced_at).filter((value): value is string => !!value).sort().at(-1)
  const emptyCompleted = emptyCategories.every((item) => item.last_outcome === 'success')
  const collapsible = emptyCategories.length >= 2
  const rows = status ? status.categories.filter((item) => !collapsible || showEmpty || !emptyCategories.includes(item)) : []
  const sum = status?.categories.reduce((total, item) => total + item.favorites, 0) ?? 0
  const running = status?.categories.filter((item) => item.state === 'running' || item.state === 'waiting_cooldown').length ?? 0
  const queued = status?.categories.filter((item) => item.queued || item.state === 'queued').length ?? 0
  const baseline = status?.downloads?.baseline_state

  useHeaderAside(status ? `${status.host} · Account ${status.account_key}` : null)

  return <Flex direction="column" gap="4" asChild><section aria-label="Favorites">
    <ResourceStatus name="Favorite statistics" plural loaded={!!status} available={available} {...favorites} />
    {status && <>
      <Grid columns={{ initial: '1', xs: '2', md: '4' }} gap="4" asChild><dl style={{ margin: 0 }}>
        {([
          ['Collected favorites', sum.toLocaleString()],
          ['Sync activity', `${running} running · ${queued} queued`, running > 0 ? 'blue' : undefined],
          ['Download baseline', baseline === 'collecting' ? `Collecting ${status.downloads?.baseline_categories} of 10` : baseline ? baselineLabels[baseline] : '—', baseline === 'ready' ? 'green' : undefined],
          status.authenticated_cooldown_until
            ? ['Authenticated requests', `Paused until ${formatDate(status.authenticated_cooldown_until)}`, 'amber']
            : ['Authenticated requests', 'No cooldown'],
        ] as [string, string, ('blue' | 'green' | 'amber')?][]).map(([label, value, color]) => <Card key={label} size="2" className={color === 'amber' ? 'collector-card-amber' : undefined} asChild><div>
          <Text as="div" size="1" color="gray" asChild><dt>{label}</dt></Text>
          <Text as="div" size="5" weight="medium" color={color} mt="1" asChild><dd style={{ margin: 0 }}>{value}</dd></Text>
        </div></Card>)}
      </dl></Grid>
      <Card size="2"><Flex align="end" wrap="wrap" gap="3" asChild><form onSubmit={(event) => { event.preventDefault(); if (!disabled) sync(category, full) }}>
        <Flex direction="column" gap="1" flexGrow="1" minWidth="220px" asChild><label>
          <Text size="1" color="gray">Category</Text>
          <Select.Root value={category} onValueChange={setCategory} disabled={disabled}><Select.Trigger aria-label="Favorite category" /><Select.Content>
            <Select.Item value="all">All ten categories</Select.Item>
            {status.categories.map((item) => <Select.Item key={item.category} value={String(item.category)}>{item.name ? `${item.category} · ${item.name}` : categoryName(item)}</Select.Item>)}
          </Select.Content></Select.Root>
        </label></Flex>
        <Flex direction="column" gap="1" asChild><fieldset disabled={disabled}>
          <Text as="div" size="1" color="gray" id="sync-mode-label">Mode</Text>
          <SegmentedControl.Root value={full ? 'full' : 'incremental'} onValueChange={(value) => setFull(value === 'full')} aria-labelledby="sync-mode-label">
            <SegmentedControl.Item value="incremental">Incremental</SegmentedControl.Item>
            <SegmentedControl.Item value="full">Full re-sync</SegmentedControl.Item>
          </SegmentedControl.Root>
        </fieldset></Flex>
        <Button size="2" variant="solid" type="submit" disabled={disabled}>{syncing ? 'Requesting…' : 'Sync favorites'}</Button>
        <Text as="p" size="1" color="gray" style={{ flex: '1 1 100%' }}>{baseline && baseline !== 'ready'
          ? 'Until the baseline is complete, sync collects all ten categories without downloading favorites.'
          : full ? 'Full re-sync reconciles removed favorites. Collected gallery references and metadata are retained.' : 'Collects the newest favorites since the last sync.'}</Text>
      </form></Flex></Card>
      {syncMutation.notice && <Text as="p" size="2" color="green" role="status">{syncMutation.notice}</Text>}
      {syncMutation.error && <Text as="p" size="2" color="red" role="alert">{syncMutation.error}</Text>}
      {downloadCategory && <MissingFavoriteDownloads key={downloadCategory.category} category={downloadCategory} available={!disabled}
        onClose={() => setDownloadCategory(null)} onSubmittingChange={setDownloadPending} />}
      <TableCard caption="Favorite categories for the current account" collectorPage="favorites" columns={[undefined, 100, 190, 150, 140, 190]} minWidth={900}>
        <Table.Header><Table.Row>
          <Table.ColumnHeaderCell scope="col">Category</Table.ColumnHeaderCell>
          <Table.ColumnHeaderCell scope="col" justify="end">Favorites</Table.ColumnHeaderCell>
          <Table.ColumnHeaderCell scope="col">Sync</Table.ColumnHeaderCell>
          <Table.ColumnHeaderCell scope="col">Last success</Table.ColumnHeaderCell>
          <Table.ColumnHeaderCell scope="col" justify="center">Auto-download</Table.ColumnHeaderCell>
          <Table.ColumnHeaderCell scope="col" justify="end"><VisuallyHidden>Actions</VisuallyHidden></Table.ColumnHeaderCell>
        </Table.Row></Table.Header>
        <Table.Body>
          {rows.map((item) => <Table.Row key={item.category} className={stateTone(item) === 'red' ? 'collector-row-failed' : undefined}>
            <Table.RowHeaderCell scope="row">{categoryName(item)}{item.name && <Text as="div" size="1" color="gray" mt="1">Category {item.category}</Text>}</Table.RowHeaderCell>
            <Table.Cell justify="end">{item.last_synced_at || item.last_saved_at ? item.favorites.toLocaleString() : '—'}</Table.Cell>
            <Table.Cell><StatusBadge tone={stateTone(item)} detail={stateDetails(item)}>{stateLabel(item)}</StatusBadge></Table.Cell>
            <Table.Cell>{item.last_synced_at ? formatDate(item.last_synced_at) : 'Never synced'}</Table.Cell>
            <Table.Cell justify="center">{status.downloads
              ? <Checkbox aria-label={`Auto-download ${categoryName(item)}`} disabled={disabled || saving} checked={selected.includes(item.category)} onCheckedChange={(checked) => {
                setDraft(checked === true ? [...selected, item.category].sort((a, b) => a - b) : selected.filter((id) => id !== item.category))
                saveMutation.setNotice('')
              }} />
              : '—'}</Table.Cell>
            <Table.Cell><RowActions slot1Width={160} slot1={<Button size="2" variant="soft" color="gray" type="button" style={{ width: '100%' }}
              disabled={disabled || downloadCategory?.category === item.category} onClick={() => setDownloadCategory(item)}>Download missing…</Button>} /></Table.Cell>
          </Table.Row>)}
          {collapsible && <Table.Row>
            <Table.RowHeaderCell scope="row">Categories {categoryRanges(emptyCategories.map((item) => item.category))}</Table.RowHeaderCell>
            <Table.Cell justify="end">0</Table.Cell>
            <Table.Cell>{emptyCompleted ? <StatusBadge tone="green">Completed</StatusBadge> : <StatusBadge tone="gray">Idle</StatusBadge>}</Table.Cell>
            <Table.Cell>{emptySynced ? formatDate(emptySynced) : '—'}</Table.Cell>
            <Table.Cell justify="center">—</Table.Cell>
            <Table.Cell><RowActions slot1Width={160} slot1={<Button size="2" variant="soft" color="gray" type="button" style={{ width: '100%' }}
              onClick={() => setShowEmpty((value) => !value)}>{showEmpty ? 'Hide' : 'Show'} {emptyCategories.length} empty</Button>} /></Table.Cell>
          </Table.Row>}
        </Table.Body>
      </TableCard>
      {status.downloads && <Flex align="center" wrap="wrap" gap="4">
        <Text size="1" color="gray" style={{ flex: '1 1 320px' }}>{status.downloads.baseline_state === 'not_started'
          ? 'Your first sync will establish a baseline across all ten categories without downloading existing favorites.'
          : status.downloads.baseline_state === 'collecting'
            ? `Establishing baseline: ${status.downloads.baseline_categories} of 10 categories complete. No automatic downloads until a later sync. Use Sync favorites to resume failed categories.`
            : 'New favorites in checked categories download automatically. Favorites from the baseline are excluded. Disabling a category leaves queued downloads running.'}</Text>
        <Button size="2" variant="soft" color="gray" type="button" disabled={disabled || saving || draft === null} onClick={() => save(selected)}>{saving ? 'Saving…' : 'Save download categories'}</Button>
      </Flex>}
      {saveMutation.notice && <Text as="p" size="2" color="green" role="status">{saveMutation.notice}</Text>}
      {saveMutation.error && <Text as="p" size="2" color="red" role="alert">{saveMutation.error}</Text>}
    </>}
  </section></Flex>
}
