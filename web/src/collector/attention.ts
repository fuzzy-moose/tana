import { collectorLink } from '../navigation'
import type { BanGroup, CollectorSummary } from './api'
import { formatDate } from './formatDate'

export interface AttentionItem {
  tone: 'amber' | 'red'
  title: string
  detail: string
  action: { label: string, href: string } | { label: string, onReset: BanGroup }
}

const count = (value: number, singular: string, many: string) => `${value.toLocaleString()} ${value === 1 ? singular : many}`

export function attentionItems(summary: CollectorSummary | undefined): AttentionItem[] {
  if (!summary) return []
  const items: AttentionItem[] = []
  const { downloads, favorites, metadata, feeds } = summary
  if (favorites.authenticated_cooldown_until) items.push({
    tone: 'amber', title: 'Authenticated cooldown',
    detail: `Favorites and archive preparation paused until ${formatDate(favorites.authenticated_cooldown_until)}`,
    action: { label: 'Reset cooldown', onReset: 'authenticated' },
  })
  if (metadata.upstream_cooldown_until) items.push({
    tone: 'amber', title: 'Main cooldown',
    detail: `Main Panda requests paused until ${formatDate(metadata.upstream_cooldown_until)}`,
    action: { label: 'Reset cooldown', onReset: 'main' },
  })
  if (downloads.storage?.paused) items.push({
    tone: 'amber', title: 'Downloads paused',
    detail: downloads.storage.reason || 'Download storage is low',
    action: { label: 'Open downloads', href: collectorLink('downloads') },
  })
  if (downloads.failed > 0) items.push({
    tone: 'red', title: `${count(downloads.failed, 'download', 'downloads')} failed`,
    detail: downloads.latest_error ? `Latest: ${downloads.latest_error}` : '',
    action: { label: 'Review failed', href: collectorLink('downloads', { state: 'failed' }) },
  })
  if (feeds.failed_captures > 0) items.push({
    tone: 'red', title: `${count(feeds.failed_captures, 'raw feed capture', 'raw feed captures')} failed`,
    detail: feeds.latest_error ?? '',
    action: { label: 'Open raw feeds', href: collectorLink('feeds', { state: 'failed' }) },
  })
  return items
}
