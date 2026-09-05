import { useState } from 'react'
import { imageURL } from './api'

export default function GalleryImage({ id, page, alt }: { id: string, page: number, alt: string }) {
  const [failed, setFailed] = useState(false)
  return failed
    ? <span className="image-placeholder">Image unavailable</span>
    : <img src={imageURL(id, page)} alt={alt} loading="lazy" decoding="async" onError={() => setFailed(true)} />
}
