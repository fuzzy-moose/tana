import { Button, Card, Flex, Grid, Heading, Text } from '@radix-ui/themes'
import { useEffect, useRef, useState } from 'react'
import { APIError } from '../api'
import type { FavoriteCategory } from './api'
import { previewMissingFavoriteDownloads, submitMissingFavoriteDownloads } from './missingFavoriteDownloads'
import type { MissingFavoriteDownloadCounts, MissingFavoriteDownloadPreview } from './missingFavoriteDownloads'

export default function MissingFavoriteDownloads({ category, available, onClose, onSubmittingChange }: {
  category: FavoriteCategory
  available: boolean
  onClose: () => void
  onSubmittingChange: (submitting: boolean) => void
}) {
  const [plan, setPlan] = useState<MissingFavoriteDownloadPreview | null>(null)
  const [result, setResult] = useState<MissingFavoriteDownloadCounts | null>(null)
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [attempted, setAttempted] = useState(false)
  const [error, setError] = useState('')
  const [revision, setRevision] = useState(0)
  const mutation = useRef<AbortController | null>(null)
  const heading = useRef<HTMLHeadingElement | null>(null)

  useEffect(() => {
    heading.current?.focus()
    return () => mutation.current?.abort()
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    previewMissingFavoriteDownloads(category.category, controller.signal).then((preview) => {
      if (!controller.signal.aborted) setPlan(preview)
    }).catch((error: Error) => {
      if (!controller.signal.aborted) setError(error.message)
    }).finally(() => {
      if (!controller.signal.aborted) setLoading(false)
    })
    return () => controller.abort()
  }, [category.category, revision])

  function refreshPreview() {
    if (!available || loading || mutation.current) return
    setPlan(null)
    setResult(null)
    setError('')
    setAttempted(false)
    setLoading(true)
    setRevision((value) => value + 1)
  }

  async function confirm() {
    if (!plan || !available || mutation.current || result) return
    const controller = new AbortController()
    mutation.current = controller
    setSubmitting(true)
    onSubmittingChange(true)
    setAttempted(true)
    setError('')
    try {
      const accepted = await submitMissingFavoriteDownloads(category.category, plan.plan_id, controller.signal)
      if (!controller.signal.aborted) setResult(accepted)
    } catch (error) {
      if (!controller.signal.aborted) {
        setError((error as Error).message)
        if (error instanceof APIError && error.code === 'missing_download_preview_invalid') setPlan(null)
      }
    } finally {
      mutation.current = null
      if (!controller.signal.aborted) {
        setSubmitting(false)
        onSubmittingChange(false)
      }
    }
  }

  const counts = result ?? plan
  const name = category.name || `Category ${category.category}`
  const blocked = !available || loading || submitting

  return <Card asChild><section aria-labelledby="missing-favorite-title"><Flex direction="column" gap="3">
    <Flex align="center" justify="between" wrap="wrap" gap="3">
      <Heading as="h3" size="3" id="missing-favorite-title" ref={heading} tabIndex={-1}>Download missing favorites · {name}</Heading>
      <Button size="2" variant="soft" type="button" disabled={submitting} onClick={onClose} color="gray">{result ? 'Close' : 'Cancel'}</Button>
    </Flex>
    <Text as="p" size="2">Uses all collected favorites in this category. Matches exact Panda IDs in source names across all registered Tana libraries, including offline libraries.</Text>
    <Text as="p" size="1" color="gray">Archives stay in the collector. Retained archives do not count as sources in a Tana library.</Text>
    {loading && <Text as="p" size="2" role="status">Checking missing favorites…</Text>}
    {error && <Text as="p" size="2" color="red" role="alert">{error}</Text>}
    {submitting && <Text as="p" size="2" role="status">Submitting reviewed downloads…</Text>}
    {result && <Text as="p" size="2" color="green" role="status">Download requests accepted. The collector will continue after you leave this page.</Text>}
    {counts && <>
      <Text as="p" size="2">{counts.total.toLocaleString()} collected favorites · {counts.present.toLocaleString()} present in Tana · {counts.missing.toLocaleString()} missing</Text>
      <Grid columns={{ initial: '2', sm: '3' }} gap="3" my="2" asChild><dl>
        <div><Text size="1" color="gray" asChild><dt>{result ? 'New downloads queued' : 'New downloads'}</dt></Text><Text size="5" weight="medium" asChild><dd style={{ margin: 0 }}>{counts.new_downloads.toLocaleString()}</dd></Text></div>
        <div><Text size="1" color="gray" asChild><dt>{result ? 'Queued or running jobs reused' : 'Queued or running jobs to reuse'}</dt></Text><Text size="5" weight="medium" asChild><dd style={{ margin: 0 }}>{counts.existing_jobs.toLocaleString()}</dd></Text></div>
        <div><Text size="1" color="gray" asChild><dt>{result ? 'Retained archives reused' : 'Retained archives to reuse'}</dt></Text><Text size="5" weight="medium" asChild><dd style={{ margin: 0 }}>{counts.retained_archives.toLocaleString()}</dd></Text></div>
        <div><Text size="1" color="gray" asChild><dt>Failed jobs skipped</dt></Text><Text size="5" weight="medium" asChild><dd style={{ margin: 0 }}>{counts.failed.toLocaleString()}</dd></Text></div>
        <div><Text size="1" color="gray" asChild><dt>Cancelled jobs skipped</dt></Text><Text size="5" weight="medium" asChild><dd style={{ margin: 0 }}>{counts.cancelled.toLocaleString()}</dd></Text></div>
        <div><Text size="1" color="gray" asChild><dt>Deleting jobs skipped</dt></Text><Text size="5" weight="medium" asChild><dd style={{ margin: 0 }}>{counts.deleting.toLocaleString()}</dd></Text></div>
      </dl></Grid>
      {(counts.failed > 0 || counts.cancelled > 0) && <Text as="p" size="1" color="gray">Retry failed or cancelled jobs explicitly from Downloads.</Text>}
      {!result && <Text as="p" size="1" color="gray">Confirmation keeps this reviewed set and skips favorites now present in Tana. Jobs created meanwhile are reused.</Text>}
      {!result && counts.missing === 0 && <Text as="p" size="2" role="status">No missing favorites in this category.</Text>}
    </>}
    {attempted && error && plan && <Text as="p" size="1" color="gray">You can safely retry this confirmation; existing requests will be reused.</Text>}
    <Flex align="center" wrap="wrap" gap="2">
      {!result && plan && plan.missing > 0 && <Button size="2" variant="solid" type="button" disabled={blocked} onClick={() => void confirm()}>
        {submitting ? 'Submitting…' : attempted ? 'Retry confirmation' : 'Confirm downloads'}
      </Button>}
      {!result && <Button size="2" variant="soft" type="button" disabled={blocked} onClick={refreshPreview} color="gray">Refresh preview</Button>}
      {(result || error || counts && counts.missing > 0) && <Button size="2" variant="soft" asChild color="gray"><a href="#/collector/downloads">View Downloads</a></Button>}
    </Flex>
  </Flex></section></Card>
}
