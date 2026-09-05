import type { Library } from '../libraries/api'
import type { ScanControls } from './useScan'
import './ScanPanel.css'

const phases = {
  idle: 'Ready to scan',
  discovering: 'Discovering sources…',
  importing: 'Importing sources…',
  completed: 'Scan complete',
  completed_with_errors: 'Scan complete with errors',
  failed: 'Scan failed',
}

export default function ScanPanel({ scan, libraries }: { scan: ScanControls, libraries: Library[] }) {
  const status = scan.status
  const scope = status?.library_id ? libraries.find((library) => library.id === status.library_id)?.name ?? 'Selected library' : 'All libraries'
  return <div className="scan-panel" aria-label="Library scanning">
    <div className="scan-heading">
      <h2>Scan libraries</h2>
      <p>Discover new sources and create galleries from their images.</p>
    </div>
    {scan.error && <p className="error-message" role="alert">{scan.error}</p>}
    <div role="status" aria-live="polite">
      <p className="scan-phase">{status ? phases[status.phase] : 'Checking scan status…'}{status && status.phase !== 'idle' && <span> · {scope}</span>}</p>
      {status && status.phase !== 'idle' && <>
        <dl className="scan-counts">
          <div><dt>Discovered</dt><dd>{status.discovered}</dd></div>
          <div><dt>Imported</dt><dd>{status.imported}</dd></div>
          <div><dt>Galleries created</dt><dd>{status.galleries_created}</dd></div>
          <div><dt>Failed sources</dt><dd>{status.failed_sources}</dd></div>
          <div><dt>Discovery errors</dt><dd>{status.discovery_errors}</dd></div>
          <div><dt>Skipped</dt><dd>{status.skipped}</dd></div>
        </dl>
        {status.phase === 'importing' && status.discovered > 0 && <progress aria-label="Source import progress" value={status.imported + status.failed_sources + status.skipped} max={status.discovered} />}
        {status.finished_at && <p className="scan-finished">Finished <time dateTime={status.finished_at}>{new Date(status.finished_at).toLocaleString()}</time></p>}
      </>}
    </div>
  </div>
}
