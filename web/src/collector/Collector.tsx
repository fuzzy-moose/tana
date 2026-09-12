import type { CollectorPage } from '../navigation'
import { useCollector } from './useCollector'
import Downloads from './Downloads'
import Favorites from './Favorites'
import Overview from './Overview'
import SitemapPage from './SitemapPage'
import ReferenceImports from './ReferenceImports'
import './Collector.css'

const pages: { page: CollectorPage, label: string, description: string }[] = [
  { page: 'overview', label: 'Overview', description: 'Follow collection inventory and Panda diagnostics.' },
  { page: 'downloads', label: 'Downloads', description: 'Manage queued downloads and archives retained on the collector.' },
  { page: 'favorites', label: 'Favorites', description: 'Sync Panda favorites and choose categories for automatic downloads.' },
  { page: 'sitemap', label: 'Sitemap', description: 'Collect Panda gallery references from sitemaps.' },
  { page: 'imports', label: 'Reference imports', description: 'Import Panda gallery references and follow their progress.' },
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

  return <section className="collector-page" aria-labelledby="collector-title">
    <div className="collector-toolbar">
      <div className="page-heading">
        <p className="eyebrow">Collector</p>
        <h1 id="collector-title">{current.label}</h1>
        <p>{current.description}</p>
      </div>
      <button className="button" type="button" disabled={collector.checking} onClick={collector.refresh}>{collector.checking ? 'Refreshing…' : 'Refresh'}</button>
    </div>

    <nav className="collector-navigation" aria-label="Collector navigation">
      {pages.map((item) => <a key={item.page} href={item.page === 'overview' ? '#/collector' : `#/collector/${item.page}`}
        aria-current={item.page === page ? 'page' : undefined}>{item.label}</a>)}
    </nav>

    <div className="collector-connection">
      <span className={`collector-badge${connected ? ' collector-badge-ok' : ''}`}>{connectionLabel}</span>
      {connection?.configured && <span>Reachable: {collector.connectionUnknown ? 'Unknown' : connection.reachable ? 'Yes' : 'No'} · API access: {collector.connectionUnknown || connection.authenticated === null ? 'Unknown' : connection.authenticated ? 'Accepted' : 'Rejected'}</span>}
    </div>
    {collector.error && <div className="error-banner" role="alert"><p>{collector.error}</p></div>}
    {connection && !connection.configured
      ? <div className="collector-panel"><h2>Connect a collector</h2><p>Configure Tana’s collector URL and API token, then restart Tana to enable collection.</p></div>
      : <>
        {page === 'overview' && <Overview {...props} />}
        {page === 'downloads' && <Downloads {...props} />}
        {page === 'favorites' && <Favorites {...props} />}
        {page === 'sitemap' && <SitemapPage {...props} />}
        {page === 'imports' && <ReferenceImports {...props} />}
      </>}
  </section>
}
