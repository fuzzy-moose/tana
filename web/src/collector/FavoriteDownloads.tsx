import { useEffect, useRef, useState } from 'react'
import { saveFavoriteDownloadSettings } from './api'
import type { FavoriteCategory, FavoriteDownloadsStatus } from './api'

export default function FavoriteDownloads({ status, categories, available, onSaved }: {
  status: FavoriteDownloadsStatus
  categories: FavoriteCategory[]
  available: boolean
  onSaved: (categories: number[]) => void
}) {
  const [draft, setDraft] = useState<number[] | null>(null)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const mutation = useRef<AbortController | null>(null)
  useEffect(() => () => mutation.current?.abort(), [])
  const selected = draft ?? status.categories

  async function save() {
    if (mutation.current) return
    const controller = new AbortController()
    mutation.current = controller
    setPending(true)
    setError('')
    setNotice('')
    try {
      const result = await saveFavoriteDownloadSettings(selected, controller.signal)
      if (!controller.signal.aborted) {
        onSaved(result.categories)
        setDraft(null)
        setNotice('Download categories saved.')
      }
    } catch (error) {
      if (!controller.signal.aborted) setError((error as Error).message)
    } finally {
      mutation.current = null
      if (!controller.signal.aborted) setPending(false)
    }
  }

  return <div className="favorite-download-settings">
    <h3>Automatic favorite downloads</h3>
    <p>{status.baseline_state === 'not_started'
      ? 'Your first sync will establish a baseline across all ten categories without downloading existing favorites.'
      : status.baseline_state === 'collecting'
        ? `Establishing baseline: ${status.baseline_categories} of 10 categories complete. No automatic downloads until a later sync. Use Sync favorites to resume failed categories.`
        : 'Baseline complete. Future syncs download newly discovered favorites in the selected categories.'}</p>
    <form onSubmit={(event) => { event.preventDefault(); if (available && !pending) void save() }}>
      <fieldset className="favorite-download-categories" disabled={!available || pending}>
        <legend>Download new favorites from</legend>
        {categories.map((category) => <label key={category.category}>
          <input type="checkbox" checked={selected.includes(category.category)} onChange={(event) => {
            setDraft(event.target.checked ? [...selected, category.category].sort((a, b) => a - b) : selected.filter((id) => id !== category.category))
            setNotice('')
          }} />
          {category.name ? `${category.category} · ${category.name}` : `Category ${category.category}`}
        </label>)}
      </fieldset>
      <p className="field-help">Enabling a category does not download previously observed favorites. Disabling it leaves queued downloads running. Archives stay in the collector.</p>
      <button className="button" type="submit" disabled={!available || pending || draft === null}>{pending ? 'Saving…' : 'Save download categories'}</button>
      {notice && <p className="collector-notice" role="status">{notice}</p>}
      {error && <p className="error-message" role="alert">{error}</p>}
    </form>
  </div>
}
