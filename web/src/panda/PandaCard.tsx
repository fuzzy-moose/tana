import { useState } from 'react'
import { Badge, Flex, Text, VisuallyHidden } from '@radix-ui/themes'
import { ClockIcon, ExclamationTriangleIcon } from '@radix-ui/react-icons'
import GalleryBox from '../galleries/GalleryBox'
import type { PandaGallery } from './api'

/** Hover details: everything the compact card leaves out. */
export function PandaDetails({ gallery }: { gallery: PandaGallery }) {
  const date = gallery.posted_at || gallery.first_seen_at
  return <Flex direction="column" gap="1">
    <Text weight="medium">{gallery.title}</Text>
    <Text>{gallery.page_count === null && 'Pages unknown · '}{date
      ? <>{gallery.posted_at ? 'Posted ' : 'First seen '}<time dateTime={date}>{new Date(date).toLocaleString()}</time></>
      : 'Publication unknown'}</Text>
    {gallery.metadata_source === 'feed' && <>
      <Text>Feed metadata · {gallery.metadata_state === 'failed' ? 'Collection failed' : 'Waiting for collection'}</Text>
      {gallery.fetch_eligible_at && <Text>Eligible from <time dateTime={gallery.fetch_eligible_at}>{new Date(gallery.fetch_eligible_at).toLocaleString()}</time>. Actual retrieval depends on collector availability.</Text>}
    </>}
  </Flex>
}

export default function PandaCard({ gallery }: { gallery: PandaGallery }) {
  const [failedImage, setFailedImage] = useState<string | null>(null)
  const failed = gallery.metadata_state === 'failed'
  return <GalleryBox href={gallery.url} target="_blank" rel="noopener noreferrer" title={gallery.title} pageCount={gallery.page_count} tags={gallery.summary_tags}
    tooltip={<PandaDetails gallery={gallery} />}
    status={gallery.metadata_source === 'feed' && <Badge variant="solid" color={failed ? 'red' : 'gray'}>
      {failed ? <ExclamationTriangleIcon /> : <ClockIcon />}<VisuallyHidden>{failed ? 'Collection failed' : 'Waiting for metadata'}</VisuallyHidden>
    </Badge>}
    cover={(image) => gallery.thumbnail_url && failedImage !== gallery.thumbnail_url
      ? <img src={gallery.thumbnail_url} alt="" loading="lazy" referrerPolicy="no-referrer" {...image} onError={() => setFailedImage(gallery.thumbnail_url)} />
      : <Text size="1" color="gray">No cover</Text>} />
}
