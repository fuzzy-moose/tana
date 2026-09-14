import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { Button, Callout, Card, Code, DataList, Flex, Link, Text, TextField } from '@radix-ui/themes'
import { ExternalLinkIcon, MagnifyingGlassIcon } from '@radix-ui/react-icons'
import { getMetadataFetch, lookupPanda, parseLookup } from './lookup'
import type { MetadataFetchJob, PandaLookupResult, PandaMetadata } from './lookup'

const metadataLabels: Record<keyof PandaMetadata, string> = {
  gid: 'Gallery ID', token: 'Token', error: 'Metadata error', title: 'Title', title_jpn: 'Japanese title',
  category: 'Category', thumb: 'Thumbnail URL', uploader: 'Uploader', posted: 'Posted (Unix seconds)',
  filecount: 'Page count', filesize: 'File size (bytes)', expunged: 'Expunged', rating: 'Rating',
  torrentcount: 'Torrent count', torrents: 'Torrents', tags: 'Tags', parent_gid: 'Parent gallery ID',
  parent_key: 'Parent token', current_gid: 'Current gallery ID', current_key: 'Current token',
  first_gid: 'First gallery ID', first_key: 'First token',
}

function Metadata({ value }: { value: PandaMetadata }) {
  return <DataList.Root className="panda-metadata" orientation={{ initial: 'vertical', sm: 'horizontal' }} size="2">{Object.entries(metadataLabels).map(([key, label]) => {
    const field = value[key as keyof PandaMetadata]
    return <DataList.Item key={key}><DataList.Label minWidth="160px">{label}</DataList.Label><DataList.Value>{key === 'torrents' && Array.isArray(field)
      ? <pre>{JSON.stringify(field, null, 2)}</pre>
      : Array.isArray(field) ? field.join(', ') || 'None'
        : typeof field === 'boolean' ? field ? 'Yes' : 'No'
          : field === null || field === undefined || field === '' ? 'None' : String(field)}</DataList.Value></DataList.Item>
  })}</DataList.Root>
}

export default function PandaLookup() {
  const [value, setValue] = useState('')
  const [result, setResult] = useState<PandaLookupResult | null>(null)
  const [pending, setPending] = useState(false)
  const [notice, setNotice] = useState('')
  const [error, setError] = useState('')
  const active = useRef<{ controller: AbortController, timer?: ReturnType<typeof setTimeout> } | null>(null)

  useEffect(() => () => {
    active.current?.controller.abort()
    clearTimeout(active.current?.timer)
  }, [])

  async function submit(event: FormEvent) {
    event.preventDefault()
    active.current?.controller.abort()
    clearTimeout(active.current?.timer)
    const operation = { controller: new AbortController(), timer: undefined as ReturnType<typeof setTimeout> | undefined }
    active.current = operation
    const signal = operation.controller.signal
    setResult(null)
    setNotice('')
    setError('')
    setPending(true)

    async function follow(job: MetadataFetchJob, gid: number) {
      if (signal.aborted) return
      const entry = job.entries.find((entry) => entry.gid === gid)
      if (entry?.status === 'successful') {
        const collected = await lookupPanda({ gid }, signal)
        if (!signal.aborted) { setResult(collected); setNotice(''); setPending(false) }
      } else if (entry?.status === 'failed' || job.status === 'completed') {
        setError(`Metadata collection failed${entry?.error ? `: ${entry.error}` : '.'}`)
        setNotice('')
        setPending(false)
      } else {
        setNotice('Collecting metadata. The collector continues if you leave this page.')
        operation.timer = setTimeout(() => {
          void getMetadataFetch(job.id, signal).then((next) => follow(next, gid)).catch(failed)
        }, 2000)
      }
    }

    function failed(error: unknown) {
      if (!signal.aborted) { setError((error as Error).message); setNotice(''); setPending(false) }
    }

    try {
      const ref = parseLookup(value)
      const next = await lookupPanda(ref, signal)
      if (signal.aborted) return
      setResult(next)
      if (next.fetch_job) await follow(next.fetch_job, next.gid)
      else setPending(false)
    } catch (error) { failed(error) }
  }

  return <Flex asChild className="panda-lookup" direction="column" gap="4"><section aria-labelledby="panda-lookup-title">
    <Card>
      <form onSubmit={(event) => void submit(event)}>
        <Flex direction="column" gap="2">
          <Text as="label" size="2" weight="medium" id="panda-lookup-title" htmlFor="panda-lookup-input">Look up gallery</Text>
          <Flex gap="2">
            <TextField.Root className="panda-lookup-input" id="panda-lookup-input" value={value} onChange={(event) => setValue(event.target.value)} placeholder="Gallery ID or Panda URL">
              <TextField.Slot><MagnifyingGlassIcon /></TextField.Slot>
            </TextField.Root>
            <Button type="submit">Look up</Button>
          </Flex>
        </Flex>
      </form>
    </Card>
    {pending && <Text as="p" size="2" color="gray" role="status">{notice || 'Looking up gallery…'}</Text>}
    {error && <Callout.Root color="red" role="alert"><Callout.Text>{error}</Callout.Text></Callout.Root>}
    {result && <Card className="panda-lookup-result"><Flex direction="column" gap="4">
      <Flex align="center" justify="between" wrap="wrap" gap="3">
        <Text size="2">Gallery ID: {result.gid} · Token: <Code>{result.token}</Code></Text>
        <Link href={result.url} target="_blank" rel="noopener noreferrer" size="2">Open on Panda <ExternalLinkIcon /></Link>
      </Flex>
      {result.unverified && <Callout.Root color="amber"><Callout.Text>Unverified reference. This token has not been confirmed by Panda.</Callout.Text></Callout.Root>}
      {!result.metadata && <Text as="p" size="2" color="gray">Metadata not collected.</Text>}
      {result.refreshed_at && <Text as="p" size="2" color="gray">Metadata retrieved: <time dateTime={result.refreshed_at}>{new Date(result.refreshed_at).toLocaleString()}</time></Text>}
      {result.metadata && <Metadata value={result.metadata} />}
    </Flex></Card>}
  </section></Flex>
}
