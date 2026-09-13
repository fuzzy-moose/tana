import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
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
  return <dl className="panda-metadata">{Object.entries(metadataLabels).map(([key, label]) => {
    const field = value[key as keyof PandaMetadata]
    return <div key={key}><dt>{label}</dt><dd>{key === 'torrents' && Array.isArray(field)
      ? <pre>{JSON.stringify(field, null, 2)}</pre>
      : Array.isArray(field) ? field.join(', ') || 'None'
        : typeof field === 'boolean' ? field ? 'Yes' : 'No'
          : field === null || field === undefined || field === '' ? 'None' : String(field)}</dd></div>
  })}</dl>
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

  return <section className="panda-lookup" aria-labelledby="panda-lookup-title">
    <form onSubmit={(event) => void submit(event)}>
      <label id="panda-lookup-title" htmlFor="panda-lookup-input">Look up gallery</label>
      <div className="button-group"><input className="text-input" id="panda-lookup-input" value={value} onChange={(event) => setValue(event.target.value)} placeholder="Gallery ID or Panda URL" />
        <button className="button" type="submit">Look up</button></div>
    </form>
    {pending && <p role="status">{notice || 'Looking up gallery…'}</p>}
    {error && <p className="error-message" role="alert">{error}</p>}
    {result && <div className="panda-lookup-result">
      <p>Gallery ID: {result.gid} · Token: <code>{result.token}</code>{' '}<a href={result.url} target="_blank" rel="noopener noreferrer">Open on Panda</a></p>
      {result.unverified && <p>Unverified reference. This token has not been confirmed by Panda.</p>}
      {!result.metadata && <p>Metadata not collected.</p>}
      {result.refreshed_at && <p>Metadata retrieved: <time dateTime={result.refreshed_at}>{new Date(result.refreshed_at).toLocaleString()}</time></p>}
      {result.metadata && <Metadata value={result.metadata} />}
    </div>}
  </section>
}
