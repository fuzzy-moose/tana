import { getSitemapStatus } from './api'
import { useCollectorResource } from './useCollectorResource'
import ResourceStatus from './ResourceStatus'
import Sitemap from './Sitemap'

export default function SitemapPage({ available, refreshKey }: { available: boolean, refreshKey: number }) {
  const sitemap = useCollectorResource(getSitemapStatus, available, refreshKey)
  return <>
    <ResourceStatus name="Sitemap statistics" loaded={!!sitemap.data} available={available} {...sitemap} />
    {sitemap.data && <Sitemap status={sitemap.data} available={available && !sitemap.stale} onChanged={sitemap.update} />}
  </>
}
