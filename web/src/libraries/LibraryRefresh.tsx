import { useEffect, useRef, useState } from 'react'
import { Box, Button, Callout, Card, Checkbox, Flex, Heading, Link, Text } from '@radix-ui/themes'
import { previewRefresh, removeMissingSources } from './refreshApi'
import type { RefreshPlan, RefreshResult } from './refreshApi'
import './SourceCleanup.css'

export default function LibraryRefresh({ libraryID }: { libraryID?: number }) {
  const [plan, setPlan] = useState<RefreshPlan | null>(null)
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [attempted, setAttempted] = useState(false)
  const [result, setResult] = useState<RefreshResult | null>(null)
  const [error, setError] = useState('')
  const [revision, setRevision] = useState(0)
  const submitting = useRef(false)

  useEffect(() => {
    const controller = new AbortController()
    previewRefresh(libraryID, controller.signal).then((next) => {
      if (controller.signal.aborted) return
      setPlan(next)
      setSelected(new Set(next.candidates.map((candidate) => candidate.source_id)))
    }).catch((error: Error) => {
      if (!controller.signal.aborted) setError(error.message)
    }).finally(() => {
      if (!controller.signal.aborted) setLoading(false)
    })
    return () => controller.abort()
  }, [libraryID, revision])

  function checkAgain() {
    setLoading(true)
    setPlan(null)
    setSelected(new Set())
    setAttempted(false)
    setResult(null)
    setError('')
    setRevision((value) => value + 1)
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
    if (!plan || attempted || submitting.current || selected.size === 0) return
    submitting.current = true
    setBusy(true)
    setAttempted(true)
    setError('')
    try {
      setResult(await removeMissingSources(plan.plan_id, [...selected]))
    } catch (error) {
      setError(error instanceof Error ? error.message : 'Could not complete library refresh.')
    } finally {
      setBusy(false)
      submitting.current = false
    }
  }

  const locked = loading || busy || attempted
  const removed = new Set(result?.removed)
  const failures = new Map(result?.failed.map((item) => [item.source_id, item.reason]))

  return <Box asChild maxWidth="1200px" mx="auto">
    <section aria-labelledby="refresh-title">
      <Flex direction="column" gap="5">
        <Link href="#/libraries" size="2">← Libraries</Link>
        <Flex justify="between" align="center" gap="3" wrap="wrap">
          <Box>
            <Heading as="h1" id="refresh-title" size="7">{libraryID === undefined ? 'Refresh all libraries' : 'Refresh library'}</Heading>
            <Text as="p" size="2" color="gray" mt="2">Remove catalog entries for archives and directories that no longer exist.</Text>
          </Box>
          <Button variant="soft" type="button" disabled={loading || busy} onClick={checkAgain}>Check again</Button>
        </Flex>
        <Callout.Root color="gray"><Callout.Text>Unavailable storage is preserved. Removal is blocked for a library when no cataloged source can be confirmed present, even if all sources were intentionally deleted.</Callout.Text></Callout.Root>
        {loading && <Text as="p" size="2" color="gray" role="status">Checking source paths…</Text>}
        {error && <Callout.Root color="red" role="alert"><Callout.Text>{error}</Callout.Text></Callout.Root>}
        {busy && <Text as="p" role="status">Rechecking storage and removing missing sources…</Text>}
        {result && <Callout.Root color={result.failed.length ? 'amber' : 'green'} role="status"><Callout.Text>
          {result.removed.length} {result.removed.length === 1 ? 'source' : 'sources'} removed. {result.failed.length} could not be removed.
        </Callout.Text></Callout.Root>}
        {attempted && !busy && <Text as="p" size="2" color="gray">Check again for a fresh preview before another removal.</Text>}

        {plan && <>
          {plan.candidates.length === 0 && <Card size="4"><Text as="p" color="gray" role="status">No missing sources eligible for removal.</Text></Card>}
          {plan.candidates.length > 0 && <>
            {!attempted && <Flex justify="between" align="center" gap="3" wrap="wrap">
              <Text weight="medium">{selected.size} {selected.size === 1 ? 'source' : 'sources'} selected</Text>
              <Flex gap="2" wrap="wrap">
                <Button variant="soft" color="gray" type="button" disabled={locked} onClick={() => setSelected(new Set(plan.candidates.map((candidate) => candidate.source_id)))}>Select all</Button>
                <Button variant="soft" color="gray" type="button" disabled={locked} onClick={() => setSelected(new Set())}>Clear selection</Button>
              </Flex>
            </Flex>}
            <Flex asChild direction="column" gap="3" m="0" p="0">
              <ul className="cleanup-candidates" aria-label="Missing sources">
                {plan.candidates.map((candidate) => <Card asChild size="3" key={candidate.source_id}>
                  <li>
                    <Flex direction="column" gap="3">
                      <Flex justify="between" align="center" gap="3" wrap="wrap">
                        <Text as="label" size="2" weight="medium"><Flex gap="2" align="center"><Checkbox checked={selected.has(candidate.source_id)} disabled={locked} onCheckedChange={() => toggle(candidate.source_id)} aria-label={`Select ${candidate.path}`} />{candidate.library_name}</Flex></Text>
                        {attempted && !busy && <Text as="p" size="2" color={failures.has(candidate.source_id) ? 'red' : 'gray'}>
                          {removed.has(candidate.source_id) ? 'Removed from catalog' : failures.has(candidate.source_id) ? `Kept: ${failures.get(candidate.source_id)}` : selected.has(candidate.source_id) ? 'Outcome unknown; check again.' : 'Excluded from removal'}
                        </Text>}
                      </Flex>
                      <Text as="p" size="2" className="cleanup-path">{candidate.path}</Text>
                      {candidate.galleries.length > 0 ? <Text asChild size="2" color="gray">
                        <ul aria-label={`Affected galleries for ${candidate.path}`}>
                          {candidate.galleries.map((gallery) => <li key={gallery.id}>
                            {gallery.title} — {gallery.deleted ? 'gallery will be deleted' : `${gallery.pages_removed} ${gallery.pages_removed === 1 ? 'page' : 'pages'} will be removed; gallery will be kept`}
                          </li>)}
                        </ul>
                      </Text> : <Text as="p" size="2" color="gray">No affected galleries.</Text>}
                    </Flex>
                  </li>
                </Card>)}
              </ul>
            </Flex>
            {!attempted && <Card size="3" role="group" aria-label="Confirm source removal">
              <Flex direction="column" gap="3">
                <Heading as="h2" size="4">Remove selected sources?</Heading>
                <Text as="p" size="2" color="gray">Their source-linked galleries, metadata, and reading progress will be lost. Independent galleries lose affected pages and remain even when empty. A renamed or moved source will be imported as new by Scan.</Text>
                <Flex gap="2" wrap="wrap">
                  <Button variant="soft" color="gray" asChild><a href="#/libraries">Cancel</a></Button>
                  <Button color="red" type="button" disabled={locked || selected.size === 0} onClick={() => void removeSelected()}>Remove {selected.size} {selected.size === 1 ? 'source' : 'sources'}</Button>
                </Flex>
              </Flex>
            </Card>}
          </>}
          {plan.skipped.length > 0 && <Card size="3" role="note" aria-label="Preserved entries">
            <Heading as="h2" size="4" mb="3">Preserved entries</Heading>
            <Flex asChild direction="column" gap="3" m="0"><ul>{plan.skipped.map((item) => <li key={`${item.library_id}:${item.source_id ?? 'library'}`}>
              <Text as="p" size="2"><strong>{item.library_name}</strong> — {item.reason}</Text>
              <Text as="p" size="1" color="gray" className="cleanup-path">{item.path}</Text>
            </li>)}</ul></Flex>
          </Card>}
        </>}
      </Flex>
    </section>
  </Box>
}
