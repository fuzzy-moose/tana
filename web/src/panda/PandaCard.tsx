import { useState } from 'react'
import type { PandaGallery } from './api'

export default function PandaCard({ gallery }: { gallery: PandaGallery }) {
  const [tooltipVisible, setTooltipVisible] = useState(false)
  const [failedImage, setFailedImage] = useState<string | null>(null)
  return <a className="gallery-card panda-card" href={gallery.url} target="_blank" rel="noopener noreferrer"
    onMouseEnter={() => setTooltipVisible(true)} onMouseLeave={() => setTooltipVisible(false)}
    onFocus={() => setTooltipVisible(true)} onBlur={() => setTooltipVisible(false)}
    onKeyDown={(event) => { if (event.key === 'Escape') setTooltipVisible(false) }}>
    <div className="gallery-cover">{gallery.thumbnail_url && failedImage !== gallery.thumbnail_url
      ? <img src={gallery.thumbnail_url} alt="" loading="lazy" referrerPolicy="no-referrer" onError={() => setFailedImage(gallery.thumbnail_url)} />
      : <span className="image-placeholder">No cover</span>}</div>
    <div className="gallery-title">
      <h2>{gallery.title}</h2>
      {tooltipVisible && <span className="gallery-title-tooltip" aria-hidden="true">{gallery.title}</span>}
    </div>
    <p>{gallery.page_count} {gallery.page_count === 1 ? 'page' : 'pages'} · <time dateTime={gallery.posted_at} title={new Date(gallery.posted_at).toLocaleString()}>{new Date(gallery.posted_at).toLocaleDateString()}</time></p>
  </a>
}
