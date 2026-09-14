import { Badge, Box, Button, Callout, Card, Flex, Grid, Heading, Inset, Link, Text } from '@radix-ui/themes'
import { ReaderIcon } from '@radix-ui/react-icons'
import { readerLink } from '../navigation'
import GalleryImage from './GalleryImage'
import { useGallery } from './useGallery'
import './Galleries.css'
import './GalleryDetail.css'

export default function GalleryDetail({ id }: { id: number }) {
  const { gallery, error, retry } = useGallery(id)
  const tagGroups = new Map<string, string[]>()
  for (const tag of gallery?.tags ?? []) {
    const values = tagGroups.get(tag.namespace) ?? []
    values.push(tag.value)
    tagGroups.set(tag.namespace, values)
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
          </Flex>
        </Grid>
        {tagGroups.size > 0 && <Card size="3" asChild>
          <section aria-labelledby="gallery-tags-title" className="gallery-detail-tag-section">
            <Heading as="h2" id="gallery-tags-title" size="3" mb="3">Tags</Heading>
            <Flex asChild direction="column" gap="3" m="0">
              <dl>
                {[...tagGroups].map(([namespace, values]) => <Grid columns={{ initial: 'minmax(0, 1fr)', sm: '100px minmax(0, 1fr)' }} gap="2" key={namespace}>
                  <Text asChild size="2" color="gray"><dt>{namespace === 'other' ? 'Other' : namespace}</dt></Text>
                  <Box asChild m="0" minWidth="0"><dd>
                    <Flex asChild gap="2" wrap="wrap" m="0" p="0"><ul className="gallery-detail-tags" aria-label={`${namespace === 'other' ? 'Other' : namespace} tags`}>
                      {values.map((value) => <Badge asChild key={value} variant="soft"><li>{value}</li></Badge>)}
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
