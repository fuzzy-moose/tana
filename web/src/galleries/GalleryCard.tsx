import { useState } from 'react'
import { galleryLink } from '../navigation'
import type { Gallery } from './api'
import GalleryImage from './GalleryImage'

export default function GalleryCard({ gallery }: { gallery: Gallery }) {
  const [tooltipVisible, setTooltipVisible] = useState(false)
  return <a className="gallery-card" href={galleryLink(gallery.id)}
    onMouseEnter={() => setTooltipVisible(true)} onMouseLeave={() => setTooltipVisible(false)}
    onFocus={() => setTooltipVisible(true)} onBlur={() => setTooltipVisible(false)}
    onKeyDown={(event) => { if (event.key === 'Escape') setTooltipVisible(false) }}>
    <div className="gallery-cover">{gallery.page_count > 0
      ? <GalleryImage id={gallery.id} page={1} alt="" />
      : <span className="image-placeholder">No pages</span>}</div>
    <div className="gallery-title">
      <h2>{gallery.title}</h2>
      {tooltipVisible && <span className="gallery-title-tooltip" aria-hidden="true">{gallery.title}</span>}
    </div>
    <p>{gallery.page_count} {gallery.page_count === 1 ? 'page' : 'pages'}</p>
  </a>
}
