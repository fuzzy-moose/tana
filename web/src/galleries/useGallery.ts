import { useEffect, useState } from 'react'
import { getGallery } from './api'
import type { GalleryDetails } from './api'

export function useGallery(id: string) {
  const [gallery, setGallery] = useState<GalleryDetails | null>(null)
  const [error, setError] = useState('')
  const [attempt, setAttempt] = useState(0)
  useEffect(() => {
    const controller = new AbortController()
    getGallery(id, controller.signal).then((result) => {
      if (!controller.signal.aborted) setGallery(result)
    }).catch((error: Error) => {
      if (!controller.signal.aborted) setError(error.message)
    })
    return () => controller.abort()
  }, [id, attempt])
  return { gallery, error, retry: () => { setError(''); setAttempt((n) => n + 1) } }
}
