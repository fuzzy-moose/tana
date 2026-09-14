import { Badge, Button, Card, Flex, Grid, Heading, Progress, Text } from '@radix-ui/themes'
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
    <Flex direction="column" gap="4" asChild><section aria-labelledby="reference-imports-title">
      <Flex align="center" justify="between" wrap="wrap" gap="3">
        <Heading as="h2" size="4" id="reference-imports-title">Import reference files</Heading>
        <Button size="2" variant="soft" type="button" disabled={!available || history.checking || history.pending} onClick={history.refresh} color="gray">Refresh imports</Button>
      </Flex>
      <Text as="p" size="2">Import gallery references from local files. Known references are reused; new references join the inventory after Panda validates them. Validation runs after other metadata collection.</Text>
      <Flex align="end" wrap="wrap" gap="3" asChild><form onSubmit={(event) => {
        event.preventDefault()
        void history.submit(files)
      }}>
        <Flex direction="column" gap="2" flexGrow="1" asChild><label>Reference files
          <input type="file" multiple accept=".txt,.csv,text/plain,text/csv" disabled={!available || history.uploading} onChange={(event) => setFiles(Array.from(event.target.files ?? []))} aria-describedby="reference-import-format" />
        </label></Flex>
        <Button size="2" variant="solid" type="submit" disabled={!available || history.uploading || files.length === 0}>{history.uploading ? 'Uploading…' : 'Upload files'}</Button>
      </form></Flex>
      <Text as="p" size="1" color="gray" id="reference-import-format">UTF-8 text · 100 MiB maximum per file · One <code>id,token</code> per line, such as <code>123,abc</code>. No header. Blank lines are ignored.</Text>
      <Text as="p" size="1" color="gray">Keep this page open until uploads are accepted. Interrupted uploads restart from the beginning. Each upload creates a new import; accepted imports continue on the collector.</Text>
      {history.transfer && <Flex direction="column" gap="3" role="status">
        <Text as="p" size="2">{history.transfer.bytes === history.transfer.size ? 'Awaiting acceptance' : 'Uploading'}: {history.transfer.filename} · {size(history.transfer.bytes)} / {size(history.transfer.size)}</Text>
        <Progress aria-label={`Upload progress for ${history.transfer.filename}`} value={history.transfer.bytes} max={history.transfer.size || 1} />
      </Flex>}
      {history.notice && <Text as="p" size="2" color="green" role="status">{history.notice}</Text>}
      {history.uploadErrors.map((error, index) => <Text as="p" size="2" color="red" key={index} role="alert">{error}</Text>)}
      {history.requestError && <Text as="p" size="2" color="red" role="alert">{history.requestError}</Text>}
      {history.error && <Text as="p" size="2" color="red" role="alert">{history.error}</Text>}
      {history.stale && <Text as="p" size="2" color="green" role="status">Import history is stale. Actions return after the collector reconnects and history refreshes.</Text>}
      {!history.loaded && !history.error && <Text as="p" size="1" color="gray">{available ? 'Loading import history…' : 'Import history is unavailable while the collector is disconnected.'}</Text>}
      {history.loaded && history.imports.length === 0 && <Text as="p" size="1" color="gray">No reference imports yet.</Text>}
      <Text as="p" size="1" color="gray">Completed imports are retained for seven days. Active imports remain until resolved.</Text>
      <Flex direction="column" gap="3">{history.imports.map((item) => {
        const outstanding = item.status === 'processing' || item.status === 'validating'
        const resolved = item.known + item.imported + item.failed + item.cancelled
        const fileState = item.status === 'processing' ? item.paused ? 'File processing paused' : 'Processing file' : item.processed_bytes < item.size_bytes ? 'File processing stopped' : 'File processed'
        return <Card key={item.id} asChild><article aria-label={`Import ${item.filename}`}><Flex direction="column" gap="3">
          <Flex align="center" justify="between" wrap="wrap" gap="3">
            <Heading as="h3" size="3">{item.filename}</Heading>
            <Badge color="gray">{item.paused ? 'Paused' : { processing: 'Processing', validating: 'Validating', completed: 'Completed', cancelled: 'Cancelled' }[item.status]}</Badge>
          </Flex>
          <Text as="p" size="1" color="gray">Accepted {date(item.created_at)}{item.completed_at && ` · Finished ${date(item.completed_at)}`}</Text>
          {item.paused && <Text as="p" size="1" color="gray">Progress is saved. Resume to continue processing and validation. Validation already underway and references found by other collection may still resolve.</Text>}
          <Grid columns={{ initial: '1', sm: '2' }} gap="4">
            <div>
              <Heading as="h4" size="3">File processing</Heading>
              <Text as="p" size="2">{fileState} · {size(item.processed_bytes)} / {size(item.size_bytes)}</Text>
              <Progress aria-label={`File processing for ${item.filename}`} value={item.processed_bytes} max={item.size_bytes || 1} />
              <Grid columns={{ initial: '2', sm: '3' }} gap="3" my="2" asChild><dl>
                <div><Text size="1" color="gray" asChild><dt>Unique references</dt></Text><Text size="5" weight="medium" asChild><dd style={{ margin: 0 }}>{item.references.toLocaleString()}</dd></Text></div>
                <div><Text size="1" color="gray" asChild><dt>Duplicates</dt></Text><Text size="5" weight="medium" asChild><dd style={{ margin: 0 }}>{item.duplicates.toLocaleString()}</dd></Text></div>
                <div><Text size="1" color="gray" asChild><dt>Invalid records</dt></Text><Text size="5" weight="medium" asChild><dd style={{ margin: 0 }}>{item.invalid.toLocaleString()}</dd></Text></div>
              </dl></Grid>
            </div>
            <div>
              <Heading as="h4" size="3">Reference validation</Heading>
              <Text as="p" size="2">{resolved.toLocaleString()} of {item.references.toLocaleString()} references resolved{item.status === 'processing' ? ' so far' : ''}</Text>
              <Progress aria-label={`Reference validation for ${item.filename}`} value={resolved} max={item.references || 1} />
              <Grid columns={{ initial: '2', sm: '3' }} gap="3" my="2" asChild><dl>
                <div><Text size="1" color="gray" asChild><dt>Already known</dt></Text><Text size="5" weight="medium" asChild><dd style={{ margin: 0 }}>{item.known.toLocaleString()}</dd></Text></div>
                <div><Text size="1" color="gray" asChild><dt>Imported</dt></Text><Text size="5" weight="medium" asChild><dd style={{ margin: 0 }}>{item.imported.toLocaleString()}</dd></Text></div>
                <div><Text size="1" color="gray" asChild><dt>Pending</dt></Text><Text size="5" weight="medium" asChild><dd style={{ margin: 0 }}>{item.pending.toLocaleString()}</dd></Text></div>
                <div><Text size="1" color="gray" asChild><dt>Failed</dt></Text><Text size="5" weight="medium" asChild><dd style={{ margin: 0 }}>{item.failed.toLocaleString()}</dd></Text></div>
                <div><Text size="1" color="gray" asChild><dt>Cancelled</dt></Text><Text size="5" weight="medium" asChild><dd style={{ margin: 0 }}>{item.cancelled.toLocaleString()}</dd></Text></div>
              </dl></Grid>
            </div>
          </Grid>
          <Flex align="center" wrap="wrap" gap="2">
            {outstanding && <Button size="2" variant="soft" type="button" disabled={actionsDisabled} onClick={() => void history.change(item.id, item.paused ? 'resume' : 'pause')} color="gray">{item.paused ? 'Resume' : 'Pause'}</Button>}
            {outstanding && <Button size="2" variant="soft" type="button" disabled={actionsDisabled} onClick={() => void history.change(item.id, 'cancel')} color="gray">Cancel</Button>}
            {item.status === 'completed' && item.failed > 0 && <Button size="2" variant="soft" type="button" disabled={actionsDisabled} onClick={() => void history.change(item.id, 'retry')} color="gray">Retry failed</Button>}
          </Flex>
        </Flex></article></Card>
      })}</Flex>
      {(history.offset > 0 || history.hasNext) && <Flex align="center" justify="end" wrap="wrap" gap="3" asChild><nav aria-label="Import history pages">
        <Button size="2" variant="soft" type="button" disabled={actionsDisabled || history.offset === 0} onClick={history.previous} color="gray">Previous imports</Button>
        <span>Page {history.offset / referenceImportPageSize + 1}</span>
        <Button size="2" variant="soft" type="button" disabled={actionsDisabled || !history.hasNext} onClick={history.next} color="gray">Next imports</Button>
      </nav></Flex>}
    </section></Flex>
  )
}
