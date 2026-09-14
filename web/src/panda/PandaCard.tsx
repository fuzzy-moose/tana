import { useState } from 'react'
import { Text, Tooltip } from '@radix-ui/themes'
import type { PandaGallery } from './api'

export default function PandaCard({ gallery }: { gallery: PandaGallery }) {
  const [failedImage, setFailedImage] = useState<string | null>(null)
  return <Tooltip content={gallery.title}><a className="gallery-card panda-card" href={gallery.url} target="_blank" rel="noopener noreferrer">
    <div className="gallery-cover">{gallery.thumbnail_url && failedImage !== gallery.thumbnail_url
      ? <img src={gallery.thumbnail_url} alt="" loading="lazy" referrerPolicy="no-referrer" onError={() => setFailedImage(gallery.thumbnail_url)} />
      : <span className="image-placeholder">No cover</span>}</div>
    <div className="gallery-title">
      <Text asChild size="1" weight="medium"><h2>{gallery.title}</h2></Text>
    </div>
    <Text as="p" size="1" color="gray">{gallery.page_count} {gallery.page_count === 1 ? 'page' : 'pages'} · <time dateTime={gallery.posted_at} title={new Date(gallery.posted_at).toLocaleString()}>{new Date(gallery.posted_at).toLocaleDateString()}</time></Text>
  </a></Tooltip>
}
