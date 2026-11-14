import { Button, Flex, Link, Text } from '@radix-ui/themes'
import { UpdateIcon } from '@radix-ui/react-icons'
import { collectorLink } from '../navigation'
import { getFavoritesStatus, syncDownloadCategories } from './api'
import { useEffect, useState } from 'react'
import { favoriteSyncView, subscribeFavoritesChanged, type FavoriteSyncActivity } from './favoriteSync'
import { formatDate } from './formatDate'
import { useCollectorResource } from './useCollectorResource'
import { useMutation } from './useMutation'

// App.tsx imports this module statically: never import Collector.tsx or a collector page component here,
// directly or through other modules, or the lazy Collector chunk ends up in the main bundle.

const ACTIVE_POLL_MS = 5000
const IDLE_POLL_MS = 30000

function activityText(activity: FavoriteSyncActivity) {
  switch (activity.kind) {
    case 'syncing': return 'Syncing'
    case 'queued': return 'Queued'
    case 'never': return 'Never synced'
    case 'waiting': return activity.at ? `Waiting for cooldown until ${formatDate(activity.at)}` : 'Waiting for cooldown'
    case 'synced': return `Last synced ${formatDate(activity.at)}`
  }
}

const unavailableReasons: Record<string, string> = {
  collector_unauthorized: 'Collector rejected access',
  collector_unavailable: 'Collector unreachable',
}

export default function FavoriteSyncAction() {
  const [fastPoll, setFastPoll] = useState(false)
  const resource = useCollectorResource(getFavoritesStatus, true, 0, fastPoll ? ACTIVE_POLL_MS : IDLE_POLL_MS, { rescheduleOnIntervalChange: true })
  const mutation = useMutation()
  const { refresh } = resource
  useEffect(() => subscribeFavoritesChanged(refresh), [refresh])
  if (resource.errorCode === 'collector_not_configured') return null
  if (!resource.data && !resource.error) return null

  const unavailable = resource.error ? unavailableReasons[resource.errorCode] ?? 'Sync status unavailable' : ''
  const view = resource.data && !unavailable ? favoriteSyncView(resource.data) : null
  const disabled = !view || view.noCategories || mutation.pending
  const nextFastPoll = !!view?.fastPoll
  if (nextFastPoll !== fastPoll) setFastPoll(nextFastPoll)

  return <Flex direction="column" gap="2" className="favorite-sync-action" aria-label="Favorite sync" role="group">
    <Button variant="soft" disabled={disabled} title={view?.title}
      onClick={() => { void mutation.run(syncDownloadCategories, { onSettled: resource.refresh }) }}>
      <UpdateIcon width="16" height="16" />
      <span className="favorite-sync-label">{mutation.pending ? 'Requesting…' : view?.label ?? 'Sync favorites'}</span>
    </Button>
    {unavailable && <Text as="p" size="1" color="gray">{unavailable}</Text>}
    {view?.noCategories && <Text as="p" size="1" color="gray">
      Check a category for auto-download on the <Link href={collectorLink('favorites')}>Favorites page</Link>
    </Text>}
    {view?.startsBaseline && <Text as="p" size="1" color="gray">Starts the baseline sync of all ten categories.</Text>}
    {view?.activity && <Text as="p" size="1" color="gray">{activityText(view.activity)}</Text>}
    {view?.progress && <Text as="p" size="1" color="gray">{view.progress}</Text>}
    {view?.failure && <Text as="p" size="1" color="red">{view.failure}</Text>}
    {mutation.error && <Text as="p" size="1" color="red" role="alert">{mutation.error}</Text>}
  </Flex>
}
