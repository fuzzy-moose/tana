import { Text } from '@radix-ui/themes'
import { galleryLink } from '../navigation'
import type { ListedGallery } from './api'
import GalleryBox from './GalleryBox'
import GalleryImage from './GalleryImage'

export default function GalleryCard({ gallery }: { gallery: ListedGallery }) {
  return <GalleryBox href={galleryLink(gallery.id)} title={gallery.title} pageCount={gallery.page_count} tags={gallery.summary_tags}
    cover={(image) => gallery.page_count > 0 ? <GalleryImage id={gallery.id} page={1} alt="" {...image} /> : <Text size="1" color="gray">No pages</Text>} />
}
