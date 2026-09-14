import { Badge, Box, Button, Callout, Flex, Heading, Select, TabNav, Text } from '@radix-ui/themes'
import type { CollectorPage } from '../navigation'
import { useCollector } from './useCollector'
import Downloads from './Downloads'
import Favorites from './Favorites'
import Overview from './Overview'
import SitemapPage from './SitemapPage'
import ReferenceImports from './ReferenceImports'
import MetadataProxies from './MetadataProxies'
import RawFeeds from './RawFeeds'
import './Collector.css'

const pages: { page: CollectorPage, label: string, description: string }[] = [
  { page: 'overview', label: 'Overview', description: 'Follow collection inventory and Panda diagnostics.' },
  { page: 'downloads', label: 'Downloads', description: 'Manage queued downloads and archives retained on the collector.' },
  { page: 'favorites', label: 'Favorites', description: 'Sync Panda favorites, download missing favorites, and choose categories for automatic downloads.' },
  { page: 'sitemap', label: 'Sitemap', description: 'Collect Panda gallery references from sitemaps.' },
  { page: 'feeds', label: 'Raw feeds', description: 'Capture Panda feeds and inspect processing failures.' },
  { page: 'imports', label: 'Reference imports', description: 'Import Panda gallery references and follow their progress.' },
  { page: 'proxies', label: 'Metadata proxies', description: 'Configure independent proxy channels for background metadata collection.' },
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
  const props = { available: connected, refreshKey: collector.revision }

  return <Flex direction="column" gap="5" className="collector-page" asChild><section aria-labelledby="collector-title">
    <Flex align="start" justify="between" gap="4" wrap="wrap">
      <Flex direction="column" gap="2">
        <Text size="1" color="gray">Collector</Text>
        <Heading size="7" id="collector-title">{current.label}</Heading>
        <Text size="2" color="gray" asChild><p className="collector-description">{current.description}</p></Text>
      </Flex>
      <Flex align="center" gap="3">
        <Badge color={connected ? 'green' : 'gray'}>{connectionLabel}</Badge>
        <Button variant="soft" disabled={collector.checking} onClick={collector.refresh} color="gray">{collector.checking ? 'Refreshing…' : 'Refresh'}</Button>
      </Flex>
    </Flex>

    <Box display={{ initial: 'none', sm: 'block' }}><TabNav.Root wrap="wrap" aria-label="Collector navigation">
      {pages.map((item) => <TabNav.Link key={item.page} href={item.page === 'overview' ? '#/collector' : `#/collector/${item.page}`}
        active={item.page === page} aria-label={item.label} aria-current={item.page === page ? 'page' : undefined}>{item.label}</TabNav.Link>)}
    </TabNav.Root></Box>
    <Box display={{ initial: 'block', sm: 'none' }}>
      <Select.Root value={page} onValueChange={(value) => { window.location.hash = value === 'overview' ? '#/collector' : `#/collector/${value}` }}>
        <Select.Trigger aria-label="Collector page" style={{ width: '100%' }} />
        <Select.Content>{pages.map((item) => <Select.Item key={item.page} value={item.page}>{item.label}</Select.Item>)}</Select.Content>
      </Select.Root>
    </Box>
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
  </section></Flex>
}
