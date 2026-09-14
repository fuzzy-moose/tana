import { useEffect, useState } from 'react'
import { Badge, Box, Button, Callout, Card, Flex, Grid, Heading, Inset, Link, Text } from '@radix-ui/themes'
import { ExternalLinkIcon, MagnifyingGlassIcon, ReaderIcon } from '@radix-ui/react-icons'
import { listingLink, readerLink } from '../navigation'
import { lookupPanda } from '../panda/lookup'
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

export default function GalleryDetail({ id }: { id: number }) {
  const { gallery, error, retry } = useGallery(id)
  const [selectedTags, setSelectedTags] = useState<string[]>([])
  const tagGroups = new Map<string, TagSuggestion[]>()
  for (const tag of gallery?.tags ?? []) {
    const values = tagGroups.get(tag.namespace) ?? []
    values.push(tag)
    tagGroups.set(tag.namespace, values)
  }
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
            <Flex asChild direction="column" gap="3" m="0">
              <dl>
                {[...tagGroups].map(([namespace, values]) => <Grid columns={{ initial: 'minmax(0, 1fr)', sm: '100px minmax(0, 1fr)' }} gap="2" key={namespace}>
                  <Text asChild size="2" color="gray"><dt>{namespace === 'other' ? 'Other' : namespace}</dt></Text>
                  <Box asChild m="0" minWidth="0"><dd>
                    <Flex asChild gap="2" wrap="wrap" m="0" p="0"><ul className="gallery-detail-tags" aria-label={`${namespace === 'other' ? 'Other' : namespace} tags`}>
                      {values.map(({ value, term }) => <li key={value}>
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
                      </li>)}
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
