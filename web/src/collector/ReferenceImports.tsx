import { Button, Card, Code, Flex, Heading, Progress, Table, Text, VisuallyHidden } from '@radix-ui/themes'
import { useState } from 'react'
import { useCollectorListParams } from '../navigation'
import CollectorPagination from './CollectorPagination'
import { useCollectorPagination } from './useCollectorPagination'
import { formatBytes } from './formatBytes'
import type { CollectorPageProps } from './pageProps'
import { referenceImportPageSize } from './referenceImports'
import type { ReferenceImport, ReferenceImportAction } from './referenceImports'
import ResourceStatus from './ResourceStatus'
import RowActions from './RowActions'
import StatusBadge from './StatusBadge'
import type { Tone } from './StatusBadge'
import TableCard from './TableCard'
import { useReferenceImports } from './useReferenceImports'
import './ReferenceImports.css'
import { formatDate } from './formatDate'

const formatCount = (value: number) => value.toLocaleString()
const statuses: Record<ReferenceImport['status'], string> = { processing: 'Processing', validating: 'Validating', completed: 'Completed', cancelled: 'Cancelled' }
const tones: Record<ReferenceImport['status'], Tone> = { processing: 'blue', validating: 'blue', completed: 'green', cancelled: 'gray' }

export default function ReferenceImports({ available, refreshKey, interval }: CollectorPageProps) {
  const { page } = useCollectorListParams()
  const history = useReferenceImports(available, refreshKey, interval, page)
  const [files, setFiles] = useState<File[]>([])
  const actionsDisabled = !available || history.pending || history.stale || history.checking || !history.loaded
  const { totalPages, page: currentPage } = useCollectorPagination({ collectorPage: 'imports', total: history.total, pageSize: referenceImportPageSize, page, loaded: history.loaded })

  return (
    <Flex direction="column" gap="4" asChild><section aria-labelledby="reference-imports-title">
      <div className="reference-imports-top">
        <Card className="reference-imports-upload"><Flex direction="column" gap="2" asChild><form onSubmit={(event) => {
          event.preventDefault()
          void history.submit(files)
        }}>
          <Text as="label" size="2" weight="medium" htmlFor="reference-files">Upload reference files</Text>
          <Flex gap="2" wrap="wrap" align="center">
            <input id="reference-files" className="reference-imports-file" type="file" multiple accept=".txt,.csv,text/plain,text/csv" disabled={!available || history.uploading} onChange={(event) => setFiles(Array.from(event.target.files ?? []))} aria-describedby="reference-format" />
            <Button size="2" variant="solid" type="submit" disabled={!available || history.uploading || files.length === 0}>{history.uploading ? 'Uploading…' : 'Upload files'}</Button>
          </Flex>
          {history.transfer && <Flex direction="column" gap="1" role="status">
            <Flex justify="between" wrap="wrap" gap="2">
              <Text size="2">{history.transfer.bytes === history.transfer.size ? 'Awaiting acceptance of' : 'Uploading'} <Text weight="medium">{history.transfer.filename}</Text></Text>
              <Text size="2" color="gray">{formatBytes(history.transfer.bytes)} / {formatBytes(history.transfer.size)}</Text>
            </Flex>
            <Progress size="1" aria-label={`Upload progress for ${history.transfer.filename}`} value={history.transfer.bytes} max={history.transfer.size || 1} />
          </Flex>}
          <Text size="1" color="gray">Keep this page open until uploads are accepted. Interrupted uploads restart from the beginning.</Text>
          {history.notice && <Text as="p" size="2" color="green" role="status">{history.notice}</Text>}
          {history.uploadErrors.map((error, index) => <Text as="p" size="2" color="red" key={index} role="alert">{error}</Text>)}
        </form></Flex></Card>
        <Card className="reference-imports-format"><Flex direction="column" gap="2" align="start" asChild><section aria-labelledby="reference-format-title" id="reference-format">
          <Heading as="h2" size="3" id="reference-format-title">File format</Heading>
          <Code size="2">123,abc</Code>
          <Text size="1" color="gray">One <Code>id,token</Code> per line · no header · blank lines ignored</Text>
          <Text size="1" color="gray">UTF-8 text · 100 MiB maximum per file · each upload creates a new import</Text>
        </section></Flex></Card>
      </div>
      {history.requestError && <Text as="p" size="2" color="red" role="alert">{history.requestError}</Text>}
      <ResourceStatus name="Import history" loaded={history.loaded} available={available} error={history.error} stale={history.stale} />
      {history.loaded && <TableCard title="Import history" titleId="reference-imports-title" caption="Reference imports · newest first" collectorPage="imports"
        aside="Completed imports are kept for 7 days · active imports until resolved" columns={[undefined, 120, 200, 240, 236]} minWidth={1000}
        empty={history.imports.length === 0 && <Text as="p" size="2" role="status">No reference imports yet.</Text>}
        footnote="Validation runs after other metadata collection. Validation already underway and references found by other collection may still resolve while an import is paused.">
        <Table.Header><Table.Row>
          <Table.ColumnHeaderCell scope="col">File</Table.ColumnHeaderCell>
          <Table.ColumnHeaderCell scope="col">Status</Table.ColumnHeaderCell>
          <Table.ColumnHeaderCell scope="col">File processing</Table.ColumnHeaderCell>
          <Table.ColumnHeaderCell scope="col">Validation</Table.ColumnHeaderCell>
          <Table.ColumnHeaderCell scope="col" justify="end"><VisuallyHidden>Actions</VisuallyHidden></Table.ColumnHeaderCell>
        </Table.Row></Table.Header>
        <Table.Body>{history.imports.map((item) => <ImportRow key={item.id} item={item} disabled={actionsDisabled} onChange={history.change} />)}</Table.Body>
      </TableCard>}
      <CollectorPagination collectorPage="imports" page={currentPage} totalPages={totalPages} label="Import history pages" />
    </section></Flex>
  )
}

function ImportRow({ item, disabled, onChange }: {
  item: ReferenceImport, disabled: boolean, onChange: (id: string, action: ReferenceImportAction) => Promise<void>,
}) {
  const outstanding = item.status === 'processing' || item.status === 'validating'
  const resolved = item.known + item.imported + item.failed + item.cancelled
  const stopped = item.processed_bytes < item.size_bytes
  const fileState = item.status === 'processing' ? item.paused ? 'File processing paused' : '' : stopped ? 'File processing stopped' : 'File processed'
  const sizes = stopped ? `${formatBytes(item.processed_bytes)} / ${formatBytes(item.size_bytes)}` : formatBytes(item.size_bytes)
  const validation = item.status === 'completed' && resolved === item.references && item.references > 0
    ? `All ${formatCount(item.references)} resolved`
    : `${formatCount(resolved)}${item.status === 'cancelled' ? '' : ` of ${formatCount(item.references)}`} resolved${item.status === 'processing' ? ' so far' : ''}`
  const slot1 = outstanding
    ? <Button size="2" variant={item.paused ? 'solid' : 'soft'} color={item.paused ? undefined : 'gray'} style={{ width: '100%' }} disabled={disabled} onClick={() => void onChange(item.id, item.paused ? 'resume' : 'pause')}>{item.paused ? 'Resume' : 'Pause'}</Button>
    : item.status === 'completed' && item.failed > 0
      ? <Button size="2" variant="soft" color="gray" style={{ width: '100%' }} disabled={disabled} onClick={() => void onChange(item.id, 'retry')}>Retry failed</Button>
      : undefined
  const slot2 = outstanding
    ? <Button size="2" variant="soft" color="gray" style={{ width: '100%' }} disabled={disabled} onClick={() => void onChange(item.id, 'cancel')}>Cancel</Button>
    : undefined

  return <Table.Row>
    <Table.RowHeaderCell scope="row">
      <span className="reference-imports-name" title={item.filename}>{item.filename}</span>
      <Text as="div" size="1" color="gray" weight="regular">Accepted {formatDate(item.created_at)}{item.completed_at && ` · finished ${formatDate(item.completed_at)}`}</Text>
    </Table.RowHeaderCell>
    <Table.Cell><StatusBadge tone={item.paused ? 'amber' : tones[item.status]}>{item.paused ? 'Paused' : statuses[item.status]}</StatusBadge></Table.Cell>
    <Table.Cell>
      <Text as="div" size="1" color="gray">{fileState ? `${fileState} · ${sizes}` : sizes}</Text>
      {outstanding && <Progress size="1" my="1" color={item.paused ? 'gray' : 'blue'} aria-label={`File processing for ${item.filename}`} value={item.processed_bytes} max={item.size_bytes || 1} />}
      <Text as="div" size="1" color="gray" mt={outstanding ? undefined : '1'}>{formatCount(item.references)} unique · {formatCount(item.duplicates)} duplicates · {formatCount(item.invalid)} invalid</Text>
    </Table.Cell>
    <Table.Cell>
      <Text as="div" size="1" color="gray">{validation}</Text>
      {outstanding && <Progress size="1" my="1" color={item.paused ? 'gray' : 'green'} aria-label={`Reference validation for ${item.filename}`} value={resolved} max={item.references || 1} />}
      <Text as="div" size="1" color="gray" mt={outstanding ? undefined : '1'}>{item.paused
        ? 'Progress saved. Resume to continue validation.'
        : <>{formatCount(item.known)} known · {formatCount(item.imported)} imported · {item.status === 'cancelled'
          ? `${formatCount(item.cancelled)} cancelled`
          : <Text color={item.failed > 0 ? 'red' : undefined}>{formatCount(item.failed)} failed</Text>}</>}</Text>
    </Table.Cell>
    <Table.Cell><RowActions slot1Width={112} slot1={slot1} slot2Width={84} slot2={slot2} /></Table.Cell>
  </Table.Row>
}
