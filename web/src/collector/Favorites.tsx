import { DotsHorizontalIcon } from '@radix-ui/react-icons'
import { Button, Card, Checkbox, DropdownMenu, Flex, Grid, IconButton, Table, Text, VisuallyHidden } from '@radix-ui/themes'
import { useState } from 'react'
import { getFavoritesStatus, saveFavoriteDownloadSettings, syncFavorites } from './api'
import type { FavoriteCategory } from './api'
import { categoryName } from './favoriteCategories'
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
import { notifyFavoritesChanged } from './favoriteSync'

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

const baselineLabels = { not_started: 'Not started', ready: 'Ready' }

export default function Favorites({ available, refreshKey, interval }: CollectorPageProps) {
  const favorites = useCollectorResource(getFavoritesStatus, available, refreshKey, interval)
  const [downloadCategory, setDownloadCategory] = useState<FavoriteCategory | null>(null)
  const [downloadPending, setDownloadPending] = useState(false)
  const [draft, setDraft] = useState<number[] | null>(null)
  const syncMutation = useMutation()
  const saveMutation = useMutation()
  const { pending: syncing } = syncMutation
  const { pending: saving } = saveMutation
  const status = favorites.data
  const disabled = !available || favorites.stale || syncing || downloadPending || !status

  // `item` is the row at click time; omitted for all ten categories.
  function sync(full: boolean, item?: FavoriteCategory) {
    if (disabled) return
    const baseline = status?.downloads?.baseline_state
    void syncMutation.run((signal) => syncFavorites(item ? String(item.category) : 'all', full, signal), {
      onSuccess: () => {
        notifyFavoritesChanged()
        if (baseline && baseline !== 'ready') return 'Baseline sync requested for all ten categories.'
        if (!item) return 'Sync requested for all ten categories.'
        if (!full) return `Sync requested for ${categoryName(item)}.`
        const suffix = item.state === 'queued' && !item.queued_full ? ' It replaces the queued sync.'
          : (item.state === 'running' || item.state === 'waiting_cooldown') && !item.full ? ' It starts after the running sync.' : ''
        return `Full re-sync requested for ${categoryName(item)}.${suffix}`
      },
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
        notifyFavoritesChanged()
        return 'Download categories saved.'
      },
    })
  }

  const selected = draft ?? status?.downloads?.categories ?? []
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
      <Card size="2"><Flex align="center" wrap="wrap" gap="3">
        <Button size="2" variant="solid" type="button" disabled={disabled} onClick={() => sync(false)}>Sync all</Button>
        <Text as="p" size="1" color="gray" style={{ flex: '1 1 320px' }}>{baseline && baseline !== 'ready'
          ? 'Until the baseline is complete, any sync collects all ten categories without downloading favorites.'
          : 'Sync collects the newest favorites since the last sync. Full re-sync, in each category\'s menu, also removes favorites no longer on Panda; collected gallery references and metadata are kept.'}</Text>
      </Flex></Card>
      {syncMutation.notice && <Text as="p" size="2" color="green" role="status">{syncMutation.notice}</Text>}
      {syncMutation.error && <Text as="p" size="2" color="red" role="alert">{syncMutation.error}</Text>}
      {downloadCategory && <MissingFavoriteDownloads key={downloadCategory.category} category={downloadCategory} available={!disabled}
        onClose={() => setDownloadCategory(null)} onSubmittingChange={setDownloadPending} />}
      <TableCard caption="Favorite categories for the current account" collectorPage="favorites" columns={[undefined, 90, 180, 150, 130, 144]} minWidth={820}>
        <Table.Header><Table.Row>
          <Table.ColumnHeaderCell scope="col">Category</Table.ColumnHeaderCell>
          <Table.ColumnHeaderCell scope="col" justify="end">Favorites</Table.ColumnHeaderCell>
          <Table.ColumnHeaderCell scope="col">Status</Table.ColumnHeaderCell>
          <Table.ColumnHeaderCell scope="col">Last success</Table.ColumnHeaderCell>
          <Table.ColumnHeaderCell scope="col" justify="center">Auto-download</Table.ColumnHeaderCell>
          <Table.ColumnHeaderCell scope="col" justify="end"><VisuallyHidden>Actions</VisuallyHidden></Table.ColumnHeaderCell>
        </Table.Row></Table.Header>
        <Table.Body>
          {status.categories.map((item) => <Table.Row key={item.category} className={stateTone(item) === 'red' ? 'collector-row-failed' : undefined}>
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
            <Table.Cell><RowActions
              slot1Width={80} slot1={<Button size="2" variant="soft" color="gray" type="button" style={{ width: '100%' }}
                disabled={disabled} onClick={() => sync(false, item)}>Sync</Button>}
              trailing={<DropdownMenu.Root>
                <DropdownMenu.Trigger disabled={disabled}><IconButton size="2" variant="soft" color="gray" aria-label={`More actions for ${categoryName(item)}`}><DotsHorizontalIcon /></IconButton></DropdownMenu.Trigger>
                <DropdownMenu.Content align="end">
                  <DropdownMenu.Item disabled={disabled} onSelect={() => sync(true, item)}>Full re-sync</DropdownMenu.Item>
                  <DropdownMenu.Item disabled={disabled || downloadCategory?.category === item.category} onSelect={() => setDownloadCategory(item)}>Download missing…</DropdownMenu.Item>
                </DropdownMenu.Content>
              </DropdownMenu.Root>} /></Table.Cell>
          </Table.Row>)}
        </Table.Body>
      </TableCard>
      {status.downloads && <Flex align="center" wrap="wrap" gap="4">
        <Text size="1" color="gray" style={{ flex: '1 1 320px' }}>{status.downloads.baseline_state === 'not_started'
          ? 'Your first sync will establish a baseline across all ten categories without downloading existing favorites.'
          : status.downloads.baseline_state === 'collecting'
            ? `Establishing baseline: ${status.downloads.baseline_categories} of 10 categories complete. No automatic downloads until a later sync. Use Sync all to resume failed categories.`
            : 'New favorites in checked categories download automatically. Favorites from the baseline are excluded. Disabling a category leaves queued downloads running.'}</Text>
        <Button size="2" variant="soft" color="gray" type="button" disabled={disabled || saving || draft === null} onClick={() => save(selected)}>{saving ? 'Saving…' : 'Save download categories'}</Button>
      </Flex>}
      {saveMutation.notice && <Text as="p" size="2" color="green" role="status">{saveMutation.notice}</Text>}
      {saveMutation.error && <Text as="p" size="2" color="red" role="alert">{saveMutation.error}</Text>}
    </>}
  </section></Flex>
}
