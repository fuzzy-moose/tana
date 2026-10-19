import { useState } from 'react'
import type { ComponentProps } from 'react'
import { imageURL } from './api'

export default function GalleryImage({ id, page, alt, className, onLoad }: { id: number, page: number, alt: string } & Pick<ComponentProps<'img'>, 'className' | 'onLoad'>) {
  const [failed, setFailed] = useState(false)
  // Browser-default decoding keeps cached covers painting reliably in Chromium.
  return failed
    ? <span className="image-placeholder">Image unavailable</span>
    : <img src={imageURL(id, page)} alt={alt} className={className} loading="lazy" onLoad={onLoad} onError={() => setFailed(true)} />
}
