import { Badge, Button, Flex, Heading, Select, Table, Text } from '@radix-ui/themes'
import { useEffect, useRef, useState } from 'react'
import { getFavoritesStatus, syncFavorites } from './api'
import type { FavoriteCategory } from './api'
import { useCollectorResource } from './useCollectorResource'
import FavoriteDownloads from './FavoriteDownloads'
import MissingFavoriteDownloads from './MissingFavoriteDownloads'
import ResourceStatus from './ResourceStatus'

function date(value?: string) { return value ? new Date(value).toLocaleString() : '—' }
function categoryName(category: FavoriteCategory) { return category.name || `Category ${category.category}` }

function stateLabel(category: FavoriteCategory) {
  if (category.state === 'waiting_cooldown') return 'Waiting for cooldown'
  if (category.state === 'running') return category.full ? 'Running full re-sync' : 'Running sync'
  if (category.state === 'queued') return category.queued_full ? 'Full re-sync queued' : 'Sync queued'
  if (category.last_outcome === 'failed') return 'Failed'
  if (category.last_outcome === 'success') return 'Completed'
  return 'Idle'
}

export default function Favorites({ available, refreshKey }: { available: boolean, refreshKey: number }) {
  const favorites = useCollectorResource(getFavoritesStatus, available, refreshKey)
  const [category, setCategory] = useState('all')
  const [full, setFull] = useState(false)
  const [pending, setPending] = useState(false)
  const [notice, setNotice] = useState('')
  const [requestError, setRequestError] = useState('')
  const [downloadCategory, setDownloadCategory] = useState<FavoriteCategory | null>(null)
  const [downloadPending, setDownloadPending] = useState(false)
  const mutation = useRef<AbortController | null>(null)
  useEffect(() => () => mutation.current?.abort(), [])
  const status = favorites.data
  const disabled = !available || favorites.stale || pending || downloadPending || !status

  async function sync(category: string, full: boolean) {
    if (mutation.current || disabled) return
    const baseline = status?.downloads?.baseline_state
    const controller = new AbortController()
    mutation.current = controller
    setPending(true)
    setNotice('')
    setRequestError('')
    try {
      await syncFavorites(category, full, controller.signal)
      if (!controller.signal.aborted) setNotice(baseline && baseline !== 'ready'
        ? 'Baseline sync requested for all ten categories.'
        : `${full ? 'Full re-sync' : 'Sync'} requested for ${category === 'all' ? 'all categories' : `category ${category}`}.`)
    } catch (error) {
      if (!controller.signal.aborted) setRequestError((error as Error).message)
    } finally {
      mutation.current = null
      if (!controller.signal.aborted) { setPending(false); favorites.refresh() }
    }
  }

  function downloadSettingsSaved(categories: number[]) {
    if (status?.downloads) favorites.update({ ...status, downloads: { ...status.downloads, categories } })
  }

  return <>
    <ResourceStatus name="Favorite statistics" loaded={!!status} available={available} {...favorites} />
    {status && (
      <Flex direction="column" gap="4">
        <Flex align="center" justify="between" wrap="wrap" gap="3">
          <Text size="2">{status.categories.reduce((sum, item) => sum + item.favorites, 0).toLocaleString()} collected favorites</Text>
          <Text size="1" color="gray">{status.host} · Account {status.account_key}</Text>
        </Flex>
        {status.authenticated_cooldown_until && <Text as="p" size="2">Authenticated Panda requests (favorites and archive preparation) paused until {date(status.authenticated_cooldown_until)}.</Text>}
        <Flex align="end" wrap="wrap" gap="3" asChild><form onSubmit={(event) => { event.preventDefault(); if (!disabled) void sync(category, full) }}>
          <Flex direction="column" gap="2" flexGrow="1" asChild><label>Favorite category
            <Select.Root value={category} onValueChange={(value) => setCategory(value)} disabled={disabled}><Select.Trigger aria-label="Favorite category" /><Select.Content>
              <Select.Item value="all">All ten categories</Select.Item>
              {status.categories.map((item) => <Select.Item key={item.category} value={String(item.category)}>{item.name ? `${item.category} · ${item.name}` : categoryName(item)}</Select.Item>)}
            </Select.Content></Select.Root>
          </label></Flex>
          <Flex direction="column" gap="2" flexGrow="1" asChild><label>Sync mode
            <Select.Root value={full ? 'full' : 'incremental'} onValueChange={(value) => setFull(value === 'full')} disabled={disabled}><Select.Trigger aria-label="Sync mode" /><Select.Content>
              <Select.Item value="incremental">Incremental sync</Select.Item>
              <Select.Item value="full">Full re-sync</Select.Item>
            </Select.Content></Select.Root>
          </label></Flex>
          <Button size="2" variant="solid" type="submit" disabled={disabled}>{pending ? 'Requesting…' : full ? 'Full re-sync favorites' : 'Sync favorites'}</Button>
        </form></Flex>
        <Text as="p" size="1" color="gray">{status.downloads && status.downloads.baseline_state !== 'ready'
          ? 'Until the baseline is complete, sync collects all ten categories without downloading favorites.'
          : full ? 'Full re-sync reconciles removed favorites. Collected gallery references and metadata are retained.' : 'Collect newest favorites.'}</Text>
        {notice && <Text as="p" size="2" color="green" role="status">{notice}</Text>}
        {requestError && <Text as="p" size="2" color="red" role="alert">{requestError}</Text>}
        {downloadCategory && <MissingFavoriteDownloads key={downloadCategory.category} category={downloadCategory} available={!disabled}
          onClose={() => setDownloadCategory(null)} onSubmittingChange={setDownloadPending} />}
        <div className="collector-table-scroll">
          <Table.Root size="1" variant="surface" className="collector-table">
            <Text size="1" color="gray" align="left" mb="2" asChild><caption>Favorite categories for the current account</caption></Text>
            <Table.Header><Table.Row><Table.ColumnHeaderCell scope="col">Category</Table.ColumnHeaderCell><Table.ColumnHeaderCell scope="col">Favorites</Table.ColumnHeaderCell><Table.ColumnHeaderCell scope="col">Sync state</Table.ColumnHeaderCell><Table.ColumnHeaderCell scope="col">Sync progress</Table.ColumnHeaderCell><Table.ColumnHeaderCell scope="col">Last successful sync</Table.ColumnHeaderCell><Table.ColumnHeaderCell scope="col">Downloads</Table.ColumnHeaderCell></Table.Row></Table.Header>
            <Table.Body>{status.categories.map((item) => (
              <Table.Row key={item.category}>
                <Table.RowHeaderCell scope="row">{categoryName(item)}{item.name && <Text as="span" size="1" color="gray" style={{ display: 'block' }} mt="1">Category {item.category}</Text>}</Table.RowHeaderCell>
                <Table.Cell>{item.last_synced_at || item.last_saved_at ? item.favorites.toLocaleString() : '—'}</Table.Cell>
                <Table.Cell><Badge color="gray">{stateLabel(item)}</Badge>
                  {item.retry_at && <Text as="span" size="1" color="gray" style={{ display: 'block' }} mt="1">Retry after {date(item.retry_at)}</Text>}
                  {item.queued && item.state !== 'queued' && <Text as="span" size="1" color="gray" style={{ display: 'block' }} mt="1">{item.queued_full ? 'Full re-sync' : 'Sync'} also queued</Text>}
                  {item.state === 'idle' && item.finished_at && <Text as="span" size="1" color="gray" style={{ display: 'block' }} mt="1">{date(item.finished_at)}</Text>}
                </Table.Cell>
                <Table.Cell>{item.started_at ? <>
                  <span>{(item.entries_saved ?? 0).toLocaleString()} entries saved · {(item.pages_saved ?? 0).toLocaleString()} pages</span>
                  <Text as="span" size="1" color="gray" style={{ display: 'block' }} mt="1">{item.last_saved_at ? `Last save ${date(item.last_saved_at)}` : 'Awaiting first page'}</Text>
                </> : '—'}</Table.Cell>
                <Table.Cell>{item.last_synced_at ? date(item.last_synced_at) : 'Never synced'}</Table.Cell>
                <Table.Cell><Button size="2" variant="soft" type="button" disabled={disabled || downloadCategory?.category === item.category} onClick={() => setDownloadCategory(item)} color="gray">Download missing…</Button></Table.Cell>
              </Table.Row>
            ))}</Table.Body>
          </Table.Root>
        </div>

        {status.downloads && <FavoriteDownloads status={status.downloads} categories={status.categories} available={!disabled} onSaved={downloadSettingsSaved} />}

        {status.categories.some((item) => item.last_error) && <>
          <Heading as="h3" size="3">Latest favorite sync errors</Heading>
          <Flex direction="column" gap="2" asChild><ul style={{ listStyle: 'none', padding: 0, margin: 0 }}>{status.categories.filter((item) => item.last_error).map((item) => <Flex direction="column" gap="1" key={item.category} asChild><li><strong>{categoryName(item)}</strong><span>{item.last_error}</span><Text size="1" color="gray">{date(item.last_error_at)}</Text></li></Flex>)}</ul></Flex>
        </>}
      </Flex>
    )}
  </>
}
