import { Text, Tooltip } from '@radix-ui/themes'
import { galleryLink } from '../navigation'
import type { Gallery } from './api'
import GalleryImage from './GalleryImage'

export default function GalleryCard({ gallery }: { gallery: Gallery }) {
  return <Tooltip content={gallery.title}><a className="gallery-card" href={galleryLink(gallery.id)}>
    <div className="gallery-cover">{gallery.page_count > 0
      ? <GalleryImage id={gallery.id} page={1} alt="" />
      : <span className="image-placeholder">No pages</span>}</div>
    <div className="gallery-title">
      <Text asChild size="1" weight="medium"><h2>{gallery.title}</h2></Text>
    </div>
    <Text as="p" size="1" color="gray">{gallery.page_count} {gallery.page_count === 1 ? 'page' : 'pages'}</Text>
  </a></Tooltip>
}
