import { useEffect, useState } from 'react'
import { Badge, Box, Button, Callout, Card, Flex, Grid, Heading, Inset, Link, Popover, Text, TextField, VisuallyHidden } from '@radix-ui/themes'
import { DrawingPinFilledIcon, ExternalLinkIcon, MagnifyingGlassIcon, QuestionMarkCircledIcon, ReaderIcon } from '@radix-ui/react-icons'
import { useMutation } from '../collector/useMutation'
import { listingLink, readerLink } from '../navigation'
import { lookupPanda } from '../panda/lookup'
import { formatWeight, getPandaTagWeights, parseNumber, removePandaScorePin, savePandaScorePin } from '../panda/score'
import type { ScoreTagWeight, ScoreTagWeights } from '../panda/score'
import type { TagSuggestion } from './api'
import GalleryImage from './GalleryImage'
import { useGallery } from './useGallery'
import './Galleries.css'
import './GalleryDetail.css'

function GalleryPandaLink({ id }: { id: number }) {
  const [url, setURL] = useState('')
  useEffect(() => {
    const controller = new AbortController()
    lookupPanda({ gid: id }, controller.signal).then((result) => {
      if (!controller.signal.aborted) setURL(result.url)
    }).catch(() => {
      // The optional link needs a reference available from the collector.
    })
    return () => controller.abort()
  }, [id])
  return url && <Link href={url} target="_blank" rel="noopener noreferrer" size="2">Open on Panda <ExternalLinkIcon /></Link>
}

/** The text the pin editor starts with: the stored pin, else the shown weight, else 0. */
function startWeight({ weight, pin }: ScoreTagWeight) {
  if (pin) return String(pin.weight)
  return weight === null ? '0' : String(Number(weight.toFixed(2)))
}

/** The weights of the gallery's tags, or null while the collector gives no valid answer. */
function useTagWeights(labels: string[]) {
  const key = JSON.stringify(labels)
  const [weights, setWeights] = useState<ScoreTagWeights | null>(null)
  const [version, setVersion] = useState(0)
  useEffect(() => {
    const tags = JSON.parse(key) as string[]
    if (tags.length === 0) return
    const controller = new AbortController()
    getPandaTagWeights(tags, controller.signal).then((answer) => {
      if (controller.signal.aborted) return
      setWeights(typeof answer?.active === 'boolean' && Array.isArray(answer.tags) ? answer : null)
    }).catch(() => {
      // The weights are optional: without the collector the tags look as they do without a score.
      if (!controller.signal.aborted) setWeights(null)
    })
    return () => controller.abort()
  }, [key, version])
  return { weights, reload: () => setVersion((n) => n + 1) }
}

/** The weight button of a tag and its pin editor. */
function TagWeight({ label, entry, onChanged }: { label: string, entry: ScoreTagWeight, onChanged: () => void }) {
  const [open, setOpen] = useState(false)
  const [text, setText] = useState('')
  const change = useMutation()
  const shown = formatWeight(entry.weight)
  const { pin } = entry
  const weight = parseNumber(text)
  async function run(operation: (signal: AbortSignal) => Promise<unknown>) {
    if (await change.run(operation)) {
      setOpen(false)
      onChanged()
    }
  }
  const pinWeight = (value: number) => run((signal) => savePandaScorePin({ kind: 'tag', value: label, weight: value }, signal))
  return <Popover.Root open={open} onOpenChange={(next) => {
    if (next) { setText(startWeight(entry)); change.clear() }
    setOpen(next)
  }}>
    <Popover.Trigger>
      <Button type="button" size="1" variant="ghost" color="gray" className="gallery-detail-weight"
        aria-label={`${pin ? 'Pinned weight' : 'Weight'} of ${label}: ${shown}`}>
        {pin && <DrawingPinFilledIcon aria-hidden />}{shown}
      </Button>
    </Popover.Trigger>
    <Popover.Content size="1" maxWidth="320px">
      <Flex direction="column" gap="2">
        <Text as="p" size="2" weight="medium">{label}</Text>
        <Flex gap="1" align="center" wrap="wrap">
          <TextField.Root size="1" type="number" step="any" aria-label="Weight" style={{ width: '6rem' }}
            value={text} onChange={(event) => setText(event.target.value)}
            onKeyDown={(event) => { if (event.key === 'Enter' && weight !== null && !change.pending) { event.preventDefault(); pinWeight(weight) } }} />
          <Button size="1" variant="soft" disabled={weight === null || change.pending} onClick={() => { if (weight !== null) pinWeight(weight) }}>Pin</Button>
          <Button size="1" variant="soft" color="gray" disabled={change.pending} onClick={() => pinWeight(0)}>Ignore</Button>
          {pin && <Button size="1" variant="soft" color="gray" disabled={change.pending} onClick={() => run((signal) => removePandaScorePin(pin.id, signal))}>Remove pin</Button>}
        </Flex>
        {change.error && <Text as="p" size="2" color="red" role="alert">Could not change the pin. {change.error}</Text>}
      </Flex>
    </Popover.Content>
  </Popover.Root>
}

export default function GalleryDetail({ id }: { id: number }) {
  const { gallery, error, retry } = useGallery(id)
  const [selectedTags, setSelectedTags] = useState<string[]>([])
  const tagGroups = new Map<string, TagSuggestion[]>()
  for (const tag of gallery?.tags ?? []) {
    const values = tagGroups.get(tag.namespace) ?? []
    values.push(tag)
    tagGroups.set(tag.namespace, values)
  }
  const { weights, reload } = useTagWeights((gallery?.tags ?? []).map((tag) => `${tag.namespace}:${tag.value}`))
  const weightEntries = new Map((weights?.tags ?? []).map((entry) => [entry.tag, entry]))
  function toggleTag(term: string) {
    setSelectedTags((selected) => selected.includes(term) ? selected.filter((tag) => tag !== term) : [...selected, term])
  }
  return <section aria-labelledby="gallery-title">
    <Flex direction="column" gap="5">
      <Link href="#/" size="2">← Galleries</Link>
      {error && <Callout.Root color="red" role="alert">
        <Callout.Text>{error}</Callout.Text>
        <Button variant="soft" color="red" onClick={retry}>Retry</Button>
      </Callout.Root>}
      {!gallery && !error && <Text as="p" size="2" color="gray" role="status">Loading gallery…</Text>}
      {gallery && <>
        <Grid columns={{ initial: '110px 1fr', sm: '200px 1fr' }} align="center" gap={{ initial: '4', sm: '6' }}>
          <Card size="1">
            <Inset clip="padding-box" className="gallery-detail-cover">
              {gallery.page_count > 0 ? <GalleryImage id={id} page={1} alt="" /> : <Text size="2" color="gray">No pages</Text>}
            </Inset>
          </Card>
          <Flex direction="column" align="start" gap="3" minWidth="0">
            <Badge color="gray">{gallery.page_count} {gallery.page_count === 1 ? 'page' : 'pages'}</Badge>
            <Heading as="h1" id="gallery-title" size={{ initial: '5', sm: '7' }} className="gallery-detail-title">{gallery.title}</Heading>
            {gallery.page_count > 0
              ? <Button asChild size="3" mt="2"><a href={readerLink(id)}><ReaderIcon />Read gallery</a></Button>
              : <Text as="p" size="2" color="gray">This gallery has no pages to read.</Text>}
            {gallery.panda_candidate_id && <GalleryPandaLink key={gallery.panda_candidate_id} id={gallery.panda_candidate_id} />}
          </Flex>
        </Grid>
        {tagGroups.size > 0 && <Card size="3" asChild>
          <section aria-labelledby="gallery-tags-title" className="gallery-detail-tag-section">
            <Flex justify="between" align="center" gap="3" wrap="wrap" mb="3">
              <Heading as="h2" id="gallery-tags-title" size="3">Tags</Heading>
              {selectedTags.length > 0 && <Flex align="center" gap="3">
                <Button asChild size="1"><a href={listingLink(selectedTags.join(' '))}><MagnifyingGlassIcon />Search selected tags ({selectedTags.length})</a></Button>
                <Button variant="ghost" color="gray" size="1" onClick={() => setSelectedTags([])}>Clear</Button>
              </Flex>}
            </Flex>
            {weights?.active === false && <Text as="p" size="2" color="gray" mb="3">No score model is active: pinned weights have no effect.</Text>}
            <Flex asChild direction="column" gap="3" m="0">
              <dl>
                {[...tagGroups].map(([namespace, values]) => <Grid columns={{ initial: 'minmax(0, 1fr)', sm: '100px minmax(0, 1fr)' }} gap="2" key={namespace}>
                  <Text asChild size="2" color="gray"><dt>{namespace === 'other' ? 'Other' : namespace}</dt></Text>
                  <Box asChild m="0" minWidth="0"><dd>
                    <Flex asChild gap="2" wrap="wrap" m="0" p="0"><ul className="gallery-detail-tags" aria-label={`${namespace === 'other' ? 'Other' : namespace} tags`}>
                      {values.map(({ value, term }) => {
                        const label = `${namespace}:${value}`
                        const entry = weightEntries.get(label)
                        return <li key={value}>
                        <Badge asChild variant={selectedTags.includes(term) ? 'solid' : 'soft'}>
                          <a className="gallery-detail-tag" href={listingLink(term)} role="button" aria-pressed={selectedTags.includes(term)}
                            title="Click to select; middle-click to search in a new tab"
                            onClick={(event) => {
                              if (event.button !== 0 || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return
                              event.preventDefault()
                              toggleTag(term)
                            }}
                            onKeyDown={(event) => {
                              if (event.key === ' ') { event.preventDefault(); event.currentTarget.click() }
                            }}>{value}</a>
                        </Badge>
                        {entry?.known && <TagWeight label={label} entry={entry} onChanged={reload} />}
                        {entry && !entry.known && <span className="gallery-detail-unknown" title="Not in the Panda catalog">
                          <QuestionMarkCircledIcon aria-hidden /><VisuallyHidden> (not in the Panda catalog)</VisuallyHidden>
                        </span>}
                      </li>
                      })}
                    </ul></Flex>
                  </dd></Box>
                </Grid>)}
              </dl>
            </Flex>
          </section>
        </Card>}
        {gallery.page_count > 0 && <Box>
          <Flex justify="between" align="center" gap="3" wrap="wrap" mb="3">
            <Heading as="h2" size="4">Pages</Heading>
            <Text size="2" color="gray">Select a page to start reading</Text>
          </Flex>
          <ol className="gallery-detail-pages" aria-label="Gallery pages">
            {Array.from({ length: gallery.page_count }, (_, index) => index + 1).map((page) => <li key={page}>
              <Card size="1" asChild>
                <a href={readerLink(id, page)} aria-label={`Read from page ${page}`}>
                  <Inset clip="padding-box" side="top" pb="current"><div className="gallery-detail-cover"><GalleryImage id={id} page={page} alt="" /></div></Inset>
                  <Text as="p" size="1" color="gray" align="center">{page}</Text>
                </a>
              </Card>
            </li>)}
          </ol>
        </Box>}
      </>}
    </Flex>
  </section>
}
