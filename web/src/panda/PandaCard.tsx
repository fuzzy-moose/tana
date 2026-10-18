import { useState } from 'react'
import { Text, Tooltip } from '@radix-ui/themes'
import type { PandaGallery } from './api'

export default function PandaCard({ gallery }: { gallery: PandaGallery }) {
  const [failedImage, setFailedImage] = useState<string | null>(null)
  const date = gallery.posted_at || gallery.first_seen_at
  return <Tooltip content={gallery.title}><a className="gallery-card panda-card" href={gallery.url} target="_blank" rel="noopener noreferrer">
    <div className="gallery-cover">{gallery.thumbnail_url && failedImage !== gallery.thumbnail_url
      ? <img src={gallery.thumbnail_url} alt="" loading="lazy" referrerPolicy="no-referrer" onError={() => setFailedImage(gallery.thumbnail_url)} />
      : <span className="image-placeholder">No cover</span>}</div>
    <div className="gallery-title">
      <Text asChild size="1" weight="medium"><h2>{gallery.title}</h2></Text>
    </div>
    <Text as="p" size="1" color="gray">{gallery.page_count === null ? 'Pages unknown' : `${gallery.page_count} ${gallery.page_count === 1 ? 'page' : 'pages'}`} · {date
      ? <>{!gallery.posted_at && 'First seen '}<time dateTime={date} title={new Date(date).toLocaleString()}>{new Date(date).toLocaleDateString()}</time></>
      : 'Publication unknown'}</Text>
    <div className="panda-feed-status">{gallery.metadata_source === 'feed' && <>
      <Text as="p" size="1" color="gray">Feed metadata</Text>
      <Text as="p" size="1" color={gallery.metadata_state === 'failed' ? 'red' : 'gray'}>{gallery.metadata_state === 'failed' ? 'Collection failed' : 'Waiting'}</Text>
      {gallery.fetch_eligible_at && <Text as="p" size="1" color="gray" title={`Earliest collection: ${new Date(gallery.fetch_eligible_at).toLocaleString()}. Actual retrieval depends on collector availability.`}>Eligible from<br /><time dateTime={gallery.fetch_eligible_at}>{new Date(gallery.fetch_eligible_at).toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', hour12: false })}</time></Text>}
    </>}</div>
  </a></Tooltip>
}
