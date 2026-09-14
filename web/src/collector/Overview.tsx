import { Flex, Grid, Heading, Text } from '@radix-ui/themes'
import { getFavoritesStatus, getInventoryStatus, getMetadataStatus } from './api'
import { useCollectorResource } from './useCollectorResource'
import ResourceStatus from './ResourceStatus'
import MetadataCollection from './MetadataCollection'

function date(value: string) { return new Date(value).toLocaleString() }

export default function Overview({ available, refreshKey }: { available: boolean, refreshKey: number }) {
  const inventory = useCollectorResource(getInventoryStatus, available, refreshKey, 30000)
  const metadata = useCollectorResource(getMetadataStatus, available, refreshKey, 30000)
  const favorites = useCollectorResource(getFavoritesStatus, available, refreshKey, 30000)
  const counts = inventory.data
  const diagnostics = metadata.data

  return <>
    <Flex direction="column" gap="4" asChild><section aria-labelledby="metadata-collection-title">
      <Heading as="h2" size="4" id="metadata-collection-title">Main metadata collection</Heading>
      <ResourceStatus name="Metadata collection status" loaded={!!diagnostics} available={available} {...metadata} />
      {diagnostics && <MetadataCollection paused={diagnostics.main_background_paused} available={available && !metadata.stale}
        onChanged={(paused) => metadata.update({ ...diagnostics, main_background_paused: paused })} />}
    </section></Flex>
    <Flex direction="column" gap="4" asChild><section aria-labelledby="inventory-title">
      <Flex align="center" justify="between" wrap="wrap" gap="3"><Heading as="h2" size="4" id="inventory-title">Collector inventory</Heading><span>Refreshes every 30 seconds</span></Flex>
      <ResourceStatus name="Inventory statistics" loaded={!!counts} available={available} {...inventory} />
      {counts && <>
        <Grid columns={{ initial: '2', sm: '4' }} gap="3" my="2" asChild><dl>
          <div><Text size="1" color="gray" asChild><dt>Gallery references</dt></Text><Text size="5" weight="medium" asChild><dd style={{ margin: 0 }}>{counts.gallery_references.toLocaleString()}</dd></Text></div>
          <div><Text size="1" color="gray" asChild><dt>Metadata available</dt></Text><Text size="5" weight="medium" asChild><dd style={{ margin: 0 }}>{counts.metadata_available.toLocaleString()}</dd></Text></div>
          <div><Text size="1" color="gray" asChild><dt>Metadata pending</dt></Text><Text size="5" weight="medium" asChild><dd style={{ margin: 0 }}>{counts.metadata_pending.toLocaleString()}</dd></Text></div>
          <div><Text size="1" color="gray" asChild><dt>Metadata failed</dt></Text><Text size="5" weight="medium" asChild><dd style={{ margin: 0 }}>{counts.metadata_failed.toLocaleString()}</dd></Text></div>
        </dl></Grid>
        <Text as="p" size="1" color="gray">Pending and failed counts cover references without retained metadata. Explicit fetch requests: {counts.fetches_pending.toLocaleString()} pending · {counts.fetches_failed.toLocaleString()} failed.</Text>
      </>}
    </section></Flex>
    <Flex direction="column" gap="4" asChild><section aria-labelledby="diagnostics-title">
      <Heading as="h2" size="4" id="diagnostics-title">Diagnostics</Heading>
      <ResourceStatus name="Authenticated Panda diagnostics" loaded={!!favorites.data} available={available} {...favorites} />
      {favorites.data && <Text as="p" size="2">{favorites.data.authenticated_cooldown_until
        ? `Authenticated Panda requests (favorites and archive preparation) paused until ${date(favorites.data.authenticated_cooldown_until)}.`
        : 'No active authenticated Panda cooldown.'}</Text>}
      {diagnostics && <>
        <Text as="p" size="2">{diagnostics.upstream_cooldown_until ? `Main unauthenticated Panda requests paused until ${date(diagnostics.upstream_cooldown_until)}.` : 'No active main unauthenticated Panda cooldown.'}</Text>
        {diagnostics.metadata_retry_at && <Text as="p" size="2">Metadata retry after {date(diagnostics.metadata_retry_at)}.</Text>}
        {diagnostics.metadata_last_error && <Text as="p" size="2" color="red">Metadata collection: {diagnostics.metadata_last_error}</Text>}
        {diagnostics.metadata_errors.length > 0 && <><Heading as="h3" size="3">Recent metadata errors</Heading><Flex direction="column" gap="2" asChild><ul style={{ listStyle: 'none', padding: 0, margin: 0 }}>{diagnostics.metadata_errors.map((item) => <Flex direction="column" gap="1" key={item.gallery_id} asChild><li><strong>Gallery {item.gallery_id}</strong><span>{item.error}</span><Text size="1" color="gray">{item.at ? date(item.at) : '—'}</Text></li></Flex>)}</ul></Flex></>}
        {!diagnostics.metadata_last_error && diagnostics.metadata_errors.length === 0 && <Text as="p" size="1" color="gray">No recent metadata errors.</Text>}
      </>}
    </section></Flex>
  </>
}
