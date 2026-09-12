import { getInventoryStatus, getMetadataStatus } from './api'
import { useCollectorResource } from './useCollectorResource'
import ResourceStatus from './ResourceStatus'

function date(value: string) { return new Date(value).toLocaleString() }

export default function Overview({ available, refreshKey }: { available: boolean, refreshKey: number }) {
  const inventory = useCollectorResource(getInventoryStatus, available, refreshKey, 30000)
  const metadata = useCollectorResource(getMetadataStatus, available, refreshKey, 30000)
  const counts = inventory.data
  const diagnostics = metadata.data

  return <>
    <section className="collector-panel" aria-labelledby="inventory-title">
      <div className="collector-section-heading"><h2 id="inventory-title">Collector inventory</h2><span>Refreshes every 30 seconds</span></div>
      <ResourceStatus name="Inventory statistics" loaded={!!counts} available={available} {...inventory} />
      {counts && <>
        <dl className="collector-metrics">
          <div><dt>Gallery references</dt><dd>{counts.gallery_references.toLocaleString()}</dd></div>
          <div><dt>Metadata available</dt><dd>{counts.metadata_available.toLocaleString()}</dd></div>
          <div><dt>Metadata pending</dt><dd>{counts.metadata_pending.toLocaleString()}</dd></div>
          <div><dt>Metadata failed</dt><dd>{counts.metadata_failed.toLocaleString()}</dd></div>
        </dl>
        <p className="field-help">Pending and failed counts cover references without retained metadata. Explicit fetch requests: {counts.fetches_pending.toLocaleString()} pending · {counts.fetches_failed.toLocaleString()} failed.</p>
      </>}
    </section>
    <section className="collector-panel" aria-labelledby="diagnostics-title">
      <h2 id="diagnostics-title">Diagnostics</h2>
      <ResourceStatus name="Metadata diagnostics" loaded={!!diagnostics} available={available} {...metadata} />
      {diagnostics && <>
        <p>{diagnostics.upstream_cooldown_until ? `Panda requests paused until ${date(diagnostics.upstream_cooldown_until)}.` : 'No active Panda cooldown.'}</p>
        {diagnostics.metadata_retry_at && <p>Metadata retry after {date(diagnostics.metadata_retry_at)}.</p>}
        {diagnostics.metadata_last_error && <p className="error-message">Metadata collection: {diagnostics.metadata_last_error}</p>}
        {diagnostics.metadata_errors.length > 0 && <><h3>Recent metadata errors</h3><ul className="collector-errors">{diagnostics.metadata_errors.map((item) => <li key={item.gallery_id}><strong>Gallery {item.gallery_id}</strong><span>{item.error}</span><small>{item.at ? date(item.at) : '—'}</small></li>)}</ul></>}
        {!diagnostics.metadata_last_error && diagnostics.metadata_errors.length === 0 && <p className="field-help">No recent metadata errors.</p>}
      </>}
    </section>
  </>
}
