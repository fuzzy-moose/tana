import { useState } from 'react'
import type { ReactNode } from 'react'
import { Box, Button, Callout, Flex, Heading, Link, Text } from '@radix-ui/themes'
import { ChevronDownIcon, ChevronUpIcon } from '@radix-ui/react-icons'
import type { CollectorPage } from '../navigation'
import { useCollector } from './useCollector'
import { useCollectorSummary } from './useCollectorSummary'
import type { CollectorPageProps } from './pageProps'
import { HeaderAsideContext } from './headerAside'
import CollectorSidebar from './CollectorSidebar'
import type { PageInfo } from './CollectorSidebar'
import Downloads from './Downloads'
import Favorites from './Favorites'
import Overview from './Overview'
import SitemapPage from './SitemapPage'
import ReferenceImports from './ReferenceImports'
import MetadataProxies from './MetadataProxies'
import RawFeeds from './RawFeeds'
import './Collector.css'

const pages: PageInfo[] = [
  { page: 'overview', label: 'Overview', group: '', interval: 30000, description: 'Collection health, inventory and Panda cooldowns.' },
  { page: 'downloads', label: 'Downloads', group: 'Acquire', interval: 5000, description: 'Queued downloads and original archives retained on the collector.' },
  { page: 'favorites', label: 'Favorites', group: 'Acquire', interval: 5000, description: 'Sync Panda favorites and choose which categories download automatically.' },
  { page: 'sitemap', label: 'Sitemap', group: 'References', interval: 5000, description: 'Collect gallery references for automatic metadata fetching. Fresh discoveries take priority over sitemap backfill.' },
  { page: 'feeds', label: 'Raw feeds', group: 'References', interval: 30000, description: <>Captures discover gallery references for metadata collection. Browse collected metadata in <Link href="#/panda">Panda</Link>.</> },
  { page: 'imports', label: 'Reference imports', group: 'References', interval: 5000, description: 'Import gallery references from local files. Known references are reused; new references join the inventory after Panda validates them.' },
  { page: 'proxies', label: 'Metadata proxies', group: 'Settings', interval: 5000, description: 'Use fixed proxies to collect background metadata and validate reference imports. Each channel has its own pacing and Panda cooldown.' },
]

export default function Collector({ page }: { page: CollectorPage }) {
  const collector = useCollector()
  const { connection } = collector
  const connected = !!connection?.status?.available && !collector.connectionUnknown
  const connectionLabel = collector.connectionUnknown ? 'Status unavailable'
    : !connection ? 'Checking connection…'
    : !connection.configured ? 'Not configured'
    : !connection.reachable ? 'Unreachable'
    : connection.authenticated === false ? 'Access rejected'
    : connected ? 'Connected' : 'Status unavailable'
  const current = pages.find((item) => item.page === page)!
  const summary = useCollectorSummary(connected, collector.revision, current.interval)
  const props: CollectorPageProps = { available: connected, refreshKey: collector.revision, interval: current.interval, summary: summary.data, summaryStale: summary.stale, refreshSummary: summary.refresh }
  // Expanded state belongs to the page it was opened on, so navigating collapses it.
  const [expandedFor, setExpandedFor] = useState<CollectorPage | null>(null)
  const expanded = expandedFor === page
  const [aside, setAside] = useState<ReactNode>(null)
  const Chevron = expanded ? ChevronUpIcon : ChevronDownIcon

  return <HeaderAsideContext.Provider value={setAside}><section aria-labelledby="collector-title" className="collector-shell">
    <div className="collector-nav-region" data-expanded={expanded}>
      <Box display={{ initial: 'block', sm: 'none' }}>
        <Button variant="soft" color="gray" className="collector-nav-toggle" aria-expanded={expanded} aria-controls="collector-sidebar"
          onClick={() => setExpandedFor(expanded ? null : page)}>
          <span>Collector · {current.label}</span><Chevron />
        </Button>
      </Box>
      <CollectorSidebar pages={pages} current={page} summary={summary.data} connectionLabel={connectionLabel} connected={connected}
        interval={current.interval} updatedAt={summary.at} checking={collector.checking} onRefresh={collector.refresh} />
    </div>
    <Flex direction="column" gap="5" className="collector-page">
      <Flex justify="between" align="end" wrap="wrap" gap="2">
        <Flex direction="column" gap="1">
          <Heading size="7" id="collector-title">{current.label}</Heading>
          <Text size="2" color="gray" asChild><p className="collector-description">{current.description}</p></Text>
        </Flex>
        {aside && <Text size="2" color="gray">{aside}</Text>}
      </Flex>
      {collector.error && <Callout.Root color="red" role="alert"><Callout.Text>{collector.error}</Callout.Text></Callout.Root>}
      {connection && !connection.configured
        ? <Callout.Root><Callout.Text>Configure Tana’s collector URL and API token, then restart Tana to enable collection.</Callout.Text></Callout.Root>
        : <>
          {page === 'overview' && <Overview {...props} />}
          {page === 'downloads' && <Downloads {...props} />}
          {page === 'favorites' && <Favorites {...props} />}
          {page === 'sitemap' && <SitemapPage {...props} />}
          {page === 'feeds' && <RawFeeds {...props} />}
          {page === 'imports' && <ReferenceImports {...props} />}
          {page === 'proxies' && <MetadataProxies {...props} />}
        </>}
    </Flex>
  </section></HeaderAsideContext.Provider>
}
