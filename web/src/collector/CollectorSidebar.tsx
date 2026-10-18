import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { Badge, Button, Text, VisuallyHidden } from '@radix-ui/themes'
import { collectorLink } from '../navigation'
import type { CollectorPage } from '../navigation'
import type { CollectorSummary } from './api'
import { attentionItems } from './attention'

export interface PageInfo {
  page: CollectorPage
  label: string
  group: string
  description: ReactNode
  interval: number
}

function statusLabel(page: CollectorPage, summary: CollectorSummary | undefined): ReactNode {
  if (!summary) return null
  switch (page) {
    case 'overview': {
      const count = attentionItems(summary).length
      return count > 0 ? <Badge size="1" color="amber" radius="full"><span aria-hidden="true">{count}</span><VisuallyHidden>{count} need attention</VisuallyHidden></Badge> : null
    }
    case 'downloads': return summary.downloads.active > 0 ? <Text size="1" color="gray">{summary.downloads.active} active</Text> : null
    case 'favorites': return summary.favorites.syncing ? <Text size="1" color="gray">Syncing</Text> : null
    case 'sitemap': {
      const { state, children_total: total, children_processed: processed } = summary.sitemap
      return state === 'running' ? <Text size="1" color="gray">{total > 0 ? Math.floor(processed * 100 / total) : 0}%</Text> : null
    }
    case 'feeds': return summary.feeds.failed_captures > 0 ? <span className="collector-failure-dot" role="img" aria-label="Has failures" /> : null
    case 'imports': return summary.reference_imports.active > 0
      ? <Text size="1" color="gray"><span aria-hidden="true">{summary.reference_imports.active}</span><VisuallyHidden>{summary.reference_imports.active} active imports</VisuallyHidden></Text> : null
    case 'proxies': return <Text size="1" color="gray">{summary.proxies.enabled ? 'On' : 'Off'}</Text>
  }
}

function useSecondsSince(at: string | undefined) {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(timer)
  }, [])
  return at ? Math.max(0, Math.floor((now - new Date(at).getTime()) / 1000)) : undefined
}

function RefreshBlock({ interval, at, checking, onRefresh }: { interval: number, at?: string, checking: boolean, onRefresh: () => void }) {
  const seconds = useSecondsSince(at)
  return <div className="collector-refresh">
    <Text size="1" color="gray">This page updates every {interval / 1000} s</Text>
    <Button variant="soft" color="gray" disabled={checking} onClick={onRefresh}>Refresh now</Button>
    <Text size="1" color="gray">{seconds === undefined ? 'Not updated yet' : `Last updated ${seconds} s ago`}</Text>
  </div>
}

export default function CollectorSidebar({ pages, current, summary, connectionLabel, connected, interval, updatedAt, checking, onRefresh }: {
  pages: PageInfo[], current: CollectorPage, summary?: CollectorSummary, connectionLabel: string, connected: boolean,
  interval: number, updatedAt?: string, checking: boolean, onRefresh: () => void,
}) {
  const groups = [...new Set(pages.map((item) => item.group))]
  return <nav id="collector-sidebar" aria-label="Collector navigation" className="collector-sidebar">
    <div className="collector-sidebar-title">
      <Text size="4" weight="bold">Collector</Text>
      <Badge size="1" color={connected ? 'green' : 'gray'} className="collector-connection"><span className="collector-connection-dot" aria-hidden="true" />{connectionLabel}</Badge>
    </div>
    {groups.map((group) => <div key={group} className="collector-nav-group">
      {group && <Text size="1" weight="medium" color="gray" className="collector-nav-group-label">{group}</Text>}
      {pages.filter((item) => item.group === group).map((item) => <a key={item.page} href={collectorLink(item.page)}
        aria-current={item.page === current ? 'page' : undefined} className="collector-nav-link">
        <Text size="2">{item.label}</Text>{statusLabel(item.page, summary)}
      </a>)}
    </div>)}
    <RefreshBlock interval={interval} at={updatedAt} checking={checking} onRefresh={onRefresh} />
  </nav>
}
