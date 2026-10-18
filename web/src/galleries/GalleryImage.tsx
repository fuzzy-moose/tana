import { useState } from 'react'
import { imageURL } from './api'

export default function GalleryImage({ id, page, alt }: { id: number, page: number, alt: string }) {
  const [failed, setFailed] = useState(false)
  // Browser-default decoding keeps cached covers painting reliably in Chromium.
  return failed
    ? <span className="image-placeholder">Image unavailable</span>
    : <img src={imageURL(id, page)} alt={alt} loading="lazy" onError={() => setFailed(true)} />
}
