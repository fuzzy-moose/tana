import { useEffect, useRef, useState } from 'react'
import { Badge, Box, Button, Callout, Card, Checkbox, Flex, Grid, Heading, Link, Text } from '@radix-ui/themes'
import { deleteSupersededSources, previewCleanup } from './cleanupApi'
import type { CleanupPlan, CleanupResult, CleanupSource } from './cleanupApi'
import './SourceCleanup.css'

function size(bytes: number) {
  if (bytes < 1024) return `${bytes.toLocaleString()} B`
  if (bytes < 1024 ** 2) return `${(bytes / 1024).toFixed(1)} KiB`
  if (bytes < 1024 ** 3) return `${(bytes / 1024 ** 2).toFixed(1)} MiB`
  return `${(bytes / 1024 ** 3).toFixed(1)} GiB`
}

function SourceDetails({ source, label }: { source: CleanupSource, label: string }) {
  return <Flex direction="column" gap="2" minWidth="0">
    <Badge color={label === 'Older archive' ? 'red' : 'green'} style={{ alignSelf: 'flex-start' }}>{label}</Badge>
    <Heading as="h3" size="3">{source.title || `Panda gallery ${source.panda_id}`}</Heading>
    <Text as="p" size="2" color="gray">{source.library_name} · Panda {source.panda_id}</Text>
    <Text as="p" size="1" className="cleanup-path">{source.path}</Text>
  </Flex>
}

export default function SourceCleanup() {
  const [plan, setPlan] = useState<CleanupPlan | null>(null)
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const [attempted, setAttempted] = useState(false)
  const [result, setResult] = useState<CleanupResult | null>(null)
  const [error, setError] = useState('')
  const [refresh, setRefresh] = useState(0)
  const submitting = useRef(false)

  useEffect(() => {
    const controller = new AbortController()
    previewCleanup(controller.signal).then((next) => {
      if (controller.signal.aborted) return
      setPlan(next)
      setSelected(new Set(next.candidates.map((candidate) => candidate.source.id)))
    }).catch((error: Error) => {
      if (!controller.signal.aborted) setError(error.message)
    }).finally(() => {
      if (!controller.signal.aborted) setLoading(false)
    })
    return () => controller.abort()
  }, [refresh])

  function refreshPreview() {
    setLoading(true)
    setPlan(null)
    setSelected(new Set())
    setConfirming(false)
    setAttempted(false)
    setResult(null)
    setError('')
    setRefresh((value) => value + 1)
  }

  function toggle(id: number) {
    setSelected((current) => {
      const next = new Set(current)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  async function removeSelected() {
    if (!plan || !confirming || attempted || submitting.current || selected.size === 0) return
    submitting.current = true
    setBusy(true)
    setAttempted(true)
    setConfirming(false)
    setError('')
    try {
      setResult(await deleteSupersededSources(plan.plan_id, [...selected]))
    } catch (error) {
      setError(error instanceof Error ? error.message : 'Could not complete cleanup.')
    } finally {
      setBusy(false)
      submitting.current = false
    }
  }

  const selectedCandidates = plan?.candidates.filter((candidate) => selected.has(candidate.source.id)) ?? []
  const totalSize = selectedCandidates.reduce((total, candidate) => total + candidate.size_bytes, 0)
  const locked = loading || busy || attempted || confirming
  const visibleCandidates = confirming ? selectedCandidates : plan?.candidates ?? []
  const failures = new Map(result?.failed.map((item) => [item.source_id, item.reason]))
  const deleted = new Set(result?.deleted)

  return <Box asChild maxWidth="1200px" mx="auto">
    <section aria-labelledby="cleanup-title">
      <Flex direction="column" gap="5">
        <Link href="#/libraries" size="2">← Libraries</Link>
        <Flex justify="between" align="center" gap="3" wrap="wrap">
          <Box>
            <Heading as="h1" id="cleanup-title" size="7">Clean up older versions</Heading>
            <Text as="p" size="2" color="gray" mt="2">Delete older archives when a newer version is present in any registered library.</Text>
          </Box>
          <Button variant="soft" type="button" disabled={loading || busy || confirming} onClick={refreshPreview}>Refresh preview</Button>
        </Flex>
        <Text as="p" size="2" color="gray">Matches use Panda IDs in filenames and collected parent references. Review each older archive and the newer source that will remain.</Text>
        {loading && <Text as="p" size="2" color="gray" role="status">Checking sources across all libraries…</Text>}
        {error && <Callout.Root color="red" role="alert"><Callout.Text>{error}</Callout.Text></Callout.Root>}
        {busy && <Text as="p" role="status">Permanently deleting selected archives…</Text>}
        {result && <Callout.Root color={result.failed.length ? 'amber' : 'green'} role="status"><Callout.Text>
          {result.deleted.length} {result.deleted.length === 1 ? 'archive' : 'archives'} deleted. {result.failed.length} could not be deleted.
        </Callout.Text></Callout.Root>}
        {attempted && !busy && <Text as="p" size="2" color="gray">Refresh the preview to check current sources before another cleanup.</Text>}

        {plan && <>
          {plan.candidates.length === 0 && <Card size="4"><Text as="p" color="gray" role="status">No eligible older archives found.</Text></Card>}
          {plan.candidates.length > 0 && <>
            {!attempted && <Flex justify="between" align="center" gap="3" wrap="wrap">
              <Text weight="medium">{selected.size} {selected.size === 1 ? 'archive' : 'archives'} selected · {size(totalSize)}</Text>
              {!confirming && <Flex gap="2" wrap="wrap">
                <Button variant="soft" color="gray" type="button" disabled={locked} onClick={() => setSelected(new Set(plan.candidates.map((candidate) => candidate.source.id)))}>Select all</Button>
                <Button variant="soft" color="gray" type="button" disabled={locked} onClick={() => setSelected(new Set())}>Clear selection</Button>
                <Button color="red" type="button" disabled={locked || selected.size === 0} onClick={() => setConfirming(true)}>Delete selected…</Button>
              </Flex>}
            </Flex>}
            {confirming && <Card size="3" role="group" aria-label="Confirm permanent deletion">
              <Flex direction="column" gap="3">
                <Heading as="h2" size="4" color="red">Permanently delete {selected.size} {selected.size === 1 ? 'archive' : 'archives'}?</Heading>
                <Text as="p" size="2">The archives listed below ({size(totalSize)}), their catalog records, and source-linked galleries will be deleted. Files will not go to trash. This cannot be undone.</Text>
                <Flex gap="2" wrap="wrap">
                  <Button autoFocus variant="soft" color="gray" type="button" onClick={() => setConfirming(false)}>Cancel</Button>
                  <Button color="red" type="button" onClick={() => void removeSelected()}>Permanently delete {selected.size} {selected.size === 1 ? 'archive' : 'archives'}</Button>
                </Flex>
              </Flex>
            </Card>}
            <Flex asChild direction="column" gap="3" m="0" p="0">
              <ul className="cleanup-candidates" aria-label={confirming ? 'Archives to permanently delete' : 'Older archives'}>
                {visibleCandidates.map(({ source, replacement, size_bytes }) => {
                  const failure = failures.get(source.id)
                  return <Card asChild size="3" key={source.id}>
                    <li>
                      <Flex direction="column" gap="4">
                        <Flex justify="between" align="center" gap="3" wrap="wrap">
                          <Text as="label" size="2"><Flex gap="2" align="center"><Checkbox checked={selected.has(source.id)} disabled={locked} onCheckedChange={() => toggle(source.id)} aria-label={`Select ${source.path}`} />{size(size_bytes)}</Flex></Text>
                          {result && <Text as="p" size="2" color={failure ? 'red' : 'gray'}>{deleted.has(source.id) ? 'Permanently deleted' : failure ? `Could not delete: ${failure}` : selected.has(source.id) ? 'Outcome unknown; refresh preview.' : 'Kept — excluded from cleanup'}</Text>}
                        </Flex>
                        <Grid columns={{ initial: '1', sm: '2' }} gap="5">
                          <SourceDetails source={source} label="Older archive" />
                          <SourceDetails source={replacement} label="Newer source to keep" />
                        </Grid>
                      </Flex>
                    </li>
                  </Card>
                })}
              </ul>
            </Flex>
          </>}
          {plan.skipped.length > 0 && <Card size="3" asChild>
            <details>
              <Text asChild size="2" weight="medium"><summary>{plan.skipped.length} {plan.skipped.length === 1 ? 'source' : 'sources'} skipped</summary></Text>
              <Flex asChild direction="column" gap="3" mt="3" mb="0"><ul>{plan.skipped.map((item) => <li key={item.source_id}>
                <Text as="p" size="1" className="cleanup-path">{item.path}</Text>
                <Text as="p" size="2" color="gray">{item.reason}</Text>
              </li>)}</ul></Flex>
            </details>
          </Card>}
        </>}
      </Flex>
    </section>
  </Box>
}
