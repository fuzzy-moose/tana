import { useEffect, useRef, useState } from 'react'
import { imageURL } from './api'
import type { PageShape } from './spreads'

export interface ReaderImage {
  shape: PageShape
  url?: string
}

export function useReaderImages(id: number, start: number, total: number) {
  const cache = useRef(new Map<number, ReaderImage>())
  const [images, setImages] = useState(new Map<number, ReaderImage>())
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    const controller = new AbortController()
    const first = Math.max(1, start - 2)
    const last = Math.min(total, start + 3)
    // Retain only nearby decoded images, even during a long reading session.
    for (const [page, image] of cache.current) {
      if (page < first || page > last) {
        if (image.url) URL.revokeObjectURL(image.url)
        cache.current.delete(page)
      }
    }
    setImages(new Map(cache.current))

    async function load(page: number) {
      let url: string | undefined
      try {
        const response = await fetch(imageURL(id, page), { signal: controller.signal })
        if (!response.ok) throw new Error('Image unavailable')
        const blob = await response.blob()
        if (controller.signal.aborted) return
        url = URL.createObjectURL(blob)
        const image = new Image()
        image.src = url
        await image.decode()
        if (!controller.signal.aborted) {
          cache.current.set(page, { url, shape: image.naturalWidth > image.naturalHeight ? 'wide' : 'portrait' })
          url = undefined // The cache now owns the object URL.
        }
      } catch {
        if (!controller.signal.aborted) cache.current.set(page, { shape: 'error' })
      } finally {
        if (url) URL.revokeObjectURL(url)
        if (!controller.signal.aborted) setImages(new Map(cache.current))
      }
    }
    // Start the current spread before its neighbours.
    for (const page of [start, start + 1, start + 2, start + 3, start - 1, start - 2]) {
      if (page >= first && page <= last && !cache.current.has(page)) void load(page)
    }
    return () => controller.abort()
  }, [id, start, total, attempt])

  useEffect(() => {
    const entries = cache.current
    return () => {
      for (const image of entries.values()) if (image.url) URL.revokeObjectURL(image.url)
      entries.clear()
    }
  }, [])

  function retry(page: number) {
    const image = cache.current.get(page)
    if (image?.url) URL.revokeObjectURL(image.url)
    cache.current.delete(page)
    setImages(new Map(cache.current))
    setAttempt((value) => value + 1)
  }
  return { images, retry }
}
