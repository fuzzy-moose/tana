import { useState } from 'react'
import { Text } from '@radix-ui/themes'
import GalleryBox from '../galleries/GalleryBox'
import type { PandaGallery } from './api'

export default function PandaCard({ gallery }: { gallery: PandaGallery }) {
  const [failedImage, setFailedImage] = useState<string | null>(null)
  const date = gallery.posted_at || gallery.first_seen_at
  return <GalleryBox href={gallery.url} target="_blank" rel="noopener noreferrer" title={gallery.title} pageCount={gallery.page_count} tags={gallery.summary_tags}
    cover={(image) => gallery.thumbnail_url && failedImage !== gallery.thumbnail_url
      ? <img src={gallery.thumbnail_url} alt="" loading="lazy" referrerPolicy="no-referrer" {...image} onError={() => setFailedImage(gallery.thumbnail_url)} />
      : <Text size="1" color="gray">No cover</Text>}>
    <div className="panda-details">
      <Text as="p" size="1" color="gray">{gallery.page_count === null && 'Pages unknown · '}{date
        ? <>{!gallery.posted_at && 'First seen '}<time dateTime={date} title={new Date(date).toLocaleString()}>{new Date(date).toLocaleDateString()}</time></>
        : 'Publication unknown'}</Text>
      {gallery.metadata_source === 'feed' && <>
        <Text as="p" size="1" color="gray">Feed metadata</Text>
        <Text as="p" size="1" color={gallery.metadata_state === 'failed' ? 'red' : 'gray'}>{gallery.metadata_state === 'failed' ? 'Collection failed' : 'Waiting'}</Text>
        {gallery.fetch_eligible_at && <Text as="p" size="1" color="gray" title={`Earliest collection: ${new Date(gallery.fetch_eligible_at).toLocaleString()}. Actual retrieval depends on collector availability.`}>Eligible from <time dateTime={gallery.fetch_eligible_at}>{new Date(gallery.fetch_eligible_at).toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', hour12: false })}</time></Text>}
      </>}
    </div>
  </GalleryBox>
}
