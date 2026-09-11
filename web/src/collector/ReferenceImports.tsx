import { useState } from 'react'
import { referenceImportPageSize } from './referenceImports'
import { useReferenceImports } from './useReferenceImports'

function date(value: string) { return new Date(value).toLocaleString() }
function size(bytes: number) { return bytes < 1024 ? `${bytes.toLocaleString()} B` : bytes < 1024 ** 2 ? `${(bytes / 1024).toFixed(1)} KiB` : `${(bytes / 1024 ** 2).toFixed(1)} MiB` }

export default function ReferenceImports({ available, refreshKey }: { available: boolean, refreshKey: number }) {
  const history = useReferenceImports(available, refreshKey)
  const [files, setFiles] = useState<File[]>([])
  const actionsDisabled = !available || history.pending || history.stale || history.checking || !history.loaded

  return (
    <section className="collector-panel" aria-labelledby="reference-imports-title">
      <div className="collector-section-heading">
        <h2 id="reference-imports-title">Panda reference imports</h2>
        <button className="button" type="button" disabled={!available || history.checking || history.pending} onClick={history.refresh}>Refresh imports</button>
      </div>
      <p>Import gallery references from local files. Known references are reused; new references join the inventory after Panda validates them. Validation runs after other metadata collection.</p>
      <form className="collector-sync reference-import-upload" onSubmit={(event) => {
        event.preventDefault()
        void history.submit(files)
      }}>
        <label className="form-field">Reference files
          <input className="text-input" type="file" multiple accept=".jsonl,.ndjson,application/x-ndjson" disabled={!available || history.uploading} onChange={(event) => setFiles(Array.from(event.target.files ?? []))} aria-describedby="reference-import-format" />
        </label>
        <button className="button button-primary" type="submit" disabled={!available || history.uploading || files.length === 0}>{history.uploading ? 'Uploading…' : 'Upload files'}</button>
      </form>
      <p id="reference-import-format" className="field-help">UTF-8 JSONL · 100 MiB maximum per file · One reference per line, such as <code>{'{"gid":123,"token":"abc"}'}</code>. Blank lines are ignored.</p>
      <p className="field-help">Keep this page open until uploads are accepted. Interrupted uploads restart from the beginning. Each upload creates a new import; accepted imports continue on the collector.</p>
      {history.transfer && <div className="reference-import-transfer" role="status">
        <p>{history.transfer.bytes === history.transfer.size ? 'Awaiting acceptance' : 'Uploading'}: {history.transfer.filename} · {size(history.transfer.bytes)} / {size(history.transfer.size)}</p>
        <progress className="sitemap-progress" aria-label={`Upload progress for ${history.transfer.filename}`} value={history.transfer.bytes} max={history.transfer.size || 1} />
      </div>}
      {history.notice && <p className="collector-notice" role="status">{history.notice}</p>}
      {history.uploadErrors.map((error, index) => <p key={index} className="error-message" role="alert">{error}</p>)}
      {history.requestError && <p className="error-message" role="alert">{history.requestError}</p>}
      {history.error && <p className="error-message" role="alert">{history.error}</p>}
      {history.stale && <p className="collector-stale" role="status">Import history is stale. Actions return after the collector reconnects and history refreshes.</p>}
      {!history.loaded && !history.error && <p className="field-help">{available ? 'Loading import history…' : 'Import history is unavailable while the collector is disconnected.'}</p>}
      {history.loaded && history.imports.length === 0 && <p className="field-help">No reference imports yet.</p>}
      <p className="field-help">Completed imports are retained for seven days. Active imports remain until resolved.</p>
      <div className="reference-import-history">{history.imports.map((item) => {
        const outstanding = item.status === 'processing' || item.status === 'validating'
        const resolved = item.known + item.imported + item.failed + item.cancelled
        const fileState = item.status === 'processing' ? 'Processing file' : item.processed_bytes < item.size_bytes ? 'File processing stopped' : 'File processed'
        return <article className="reference-import" key={item.id} aria-label={`Import ${item.filename}`}>
          <div className="collector-section-heading">
            <h3>{item.filename}</h3>
            <span className="collector-badge">{{ processing: 'Processing', validating: 'Validating', completed: 'Completed', cancelled: 'Cancelled' }[item.status]}</span>
          </div>
          <p className="field-help">Accepted {date(item.created_at)}{item.completed_at && ` · Finished ${date(item.completed_at)}`}</p>
          <div className="reference-import-stages">
            <div>
              <h4>File processing</h4>
              <p>{fileState} · {size(item.processed_bytes)} / {size(item.size_bytes)}</p>
              <progress className="sitemap-progress" aria-label={`File processing for ${item.filename}`} value={item.processed_bytes} max={item.size_bytes || 1} />
              <dl className="reference-import-counts">
                <div><dt>Unique references</dt><dd>{item.references.toLocaleString()}</dd></div>
                <div><dt>Duplicates</dt><dd>{item.duplicates.toLocaleString()}</dd></div>
                <div><dt>Invalid records</dt><dd>{item.invalid.toLocaleString()}</dd></div>
              </dl>
            </div>
            <div>
              <h4>Reference validation</h4>
              <p>{resolved.toLocaleString()} of {item.references.toLocaleString()} references resolved{item.status === 'processing' ? ' so far' : ''}</p>
              <progress className="sitemap-progress" aria-label={`Reference validation for ${item.filename}`} value={resolved} max={item.references || 1} />
              <dl className="reference-import-counts">
                <div><dt>Already known</dt><dd>{item.known.toLocaleString()}</dd></div>
                <div><dt>Imported</dt><dd>{item.imported.toLocaleString()}</dd></div>
                <div><dt>Pending</dt><dd>{item.pending.toLocaleString()}</dd></div>
                <div><dt>Failed</dt><dd>{item.failed.toLocaleString()}</dd></div>
                <div><dt>Cancelled</dt><dd>{item.cancelled.toLocaleString()}</dd></div>
              </dl>
            </div>
          </div>
          <div className="button-group">
            {outstanding && <button className="button" type="button" disabled={actionsDisabled} onClick={() => void history.change(item.id, 'cancel')}>Cancel</button>}
            {item.status === 'completed' && item.failed > 0 && <button className="button" type="button" disabled={actionsDisabled} onClick={() => void history.change(item.id, 'retry')}>Retry failed</button>}
          </div>
        </article>
      })}</div>
      {(history.offset > 0 || history.hasNext) && <nav className="download-pagination" aria-label="Import history pages">
        <button className="button" type="button" disabled={actionsDisabled || history.offset === 0} onClick={history.previous}>Previous imports</button>
        <span>Page {history.offset / referenceImportPageSize + 1}</span>
        <button className="button" type="button" disabled={actionsDisabled || !history.hasNext} onClick={history.next}>Next imports</button>
      </nav>}
    </section>
  )
}
