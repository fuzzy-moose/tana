import { useCallback, useState } from 'react'
import { feedCaptureFileURL, getFeedCaptures } from './api'
import ResourceStatus from './ResourceStatus'
import { useCollectorResource } from './useCollectorResource'

const pageSize = 25
const states = { pending: 'Pending', processed: 'Processed', failed: 'Failed' }

export default function RawFeeds({ available, refreshKey }: { available: boolean, refreshKey: number }) {
  const [failedOnly, setFailedOnly] = useState(false)
  const [offset, setOffset] = useState(0)

  return <section className="collector-panel" aria-labelledby="feed-captures-title">
    <h2 id="feed-captures-title">Feed captures</h2>
    <p>Download the original feed bytes, including malformed XML. Capture IDs match collector logs. Downloads remain stored on the collector.</p>
    <div className="collector-sync">
      <label className="form-field">Show captures
        <select className="text-input" value={failedOnly ? 'failed' : 'all'} onChange={(event) => {
          setFailedOnly(event.target.value === 'failed')
          setOffset(0)
        }}>
          <option value="all">All captures</option>
          <option value="failed">Failed only</option>
        </select>
      </label>
    </div>
    <CaptureList key={`${failedOnly}-${offset}`} available={available} refreshKey={refreshKey}
      failedOnly={failedOnly} offset={offset} setOffset={setOffset} />
  </section>
}

function CaptureList({ available, refreshKey, failedOnly, offset, setOffset }: {
  available: boolean, refreshKey: number, failedOnly: boolean, offset: number, setOffset: (offset: number) => void,
}) {
  const load = useCallback((signal: AbortSignal) => getFeedCaptures(failedOnly, pageSize, offset, signal), [failedOnly, offset])
  const resource = useCollectorResource(load, available, refreshKey, 30000)
  const { data } = resource

  return <>
    <ResourceStatus name="Feed captures" loaded={!!data} available={available} {...resource} />
    {data?.captures.length === 0 && <p role="status">{failedOnly ? 'No failed feed captures.' : 'No feed captures.'}</p>}
    {!!data?.captures.length && <div className="collector-table-scroll">
      <table className="collector-table collector-feeds">
        <caption className="collector-table-caption">{failedOnly ? 'Failed captures' : 'All captures'} · newest first</caption>
        <thead><tr><th scope="col">Capture</th><th scope="col">Captured</th><th scope="col">Processing</th><th scope="col">Size</th><th scope="col">Download</th></tr></thead>
        <tbody>{data.captures.map((capture) => <tr key={capture.id}>
          <th scope="row">Capture {capture.id}</th>
          <td>{new Date(capture.captured_at).toLocaleString()}</td>
          <td><span className="collector-state">{states[capture.state]}</span>
            {capture.error && <span className="collector-secondary">{capture.error}</span>}
          </td>
          <td>{capture.size_bytes.toLocaleString()} B</td>
          <td>{available
            ? <a className="button" href={feedCaptureFileURL(capture.id)} download aria-label={`Download raw feed ${capture.id}`}>Download raw feed</a>
            : <button className="button" disabled>Download raw feed</button>}
          </td>
        </tr>)}</tbody>
      </table>
    </div>}
    {(offset > 0 || data?.has_more) && <nav className="download-pagination" aria-label="Feed capture pages">
      <button className="button" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - pageSize))}>Previous captures</button>
      <span>Page {offset / pageSize + 1}</span>
      <button className="button" disabled={!available || resource.checking || resource.stale || !data?.has_more} onClick={() => setOffset(offset + pageSize)}>Next captures</button>
    </nav>}
  </>
}
