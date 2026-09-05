import { useEffect, useRef, useState } from 'react'
import { checkLibraryAvailability, createLibrary, listLibraries, removeLibrary, renameLibrary } from './api'
import type { Library } from './api'
import LibraryForm from './LibraryForm'
import './Libraries.css'

type Editor = { type: 'add' } | { type: 'rename' | 'remove', library: Library }

const availabilityLabels = {
  available: 'Available',
  unavailable: 'Unavailable',
  unknown: 'Not yet checked',
}

export default function Libraries() {
  const [libraries, setLibraries] = useState<Library[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [editor, setEditor] = useState<Editor | null>(null)
  const [busy, setBusy] = useState<string | null>(null)
  const [removeError, setRemoveError] = useState('')
  const [refresh, setRefresh] = useState(0)
  const checkController = useRef<AbortController | null>(null)
  const editorTrigger = useRef<HTMLButtonElement | null>(null)
  const addButton = useRef<HTMLButtonElement | null>(null)

  useEffect(() => {
    const controller = new AbortController()
    listLibraries(controller.signal)
      .then((result) => {
        if (!controller.signal.aborted) setLibraries(result)
      })
      .catch((error: Error) => {
        if (!controller.signal.aborted) setError(error.message)
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [refresh])

  useEffect(() => () => checkController.current?.abort(), [])

  const locked = loading || busy !== null || editor !== null

  function refreshLibraries() {
    setLoading(true)
    setError('')
    setNotice('')
    setRefresh((value) => value + 1)
  }

  function openEditor(next: Editor, trigger: HTMLButtonElement) {
    editorTrigger.current = trigger
    setNotice('')
    setRemoveError('')
    setEditor(next)
  }

  function closeEditor(removed = false) {
    setEditor(null)
    // Wait for the editor to unmount and its trigger to become enabled again.
    requestAnimationFrame(() => {
      const trigger = editorTrigger.current
      const target = removed || !trigger?.isConnected ? addButton.current : trigger
      target?.focus()
    })
  }

  function updateLibrary(library: Library) {
    setLibraries((current) => current.map((item) => item.id === library.id ? library : item))
  }

  async function save(name: string, path: string) {
    if (editor?.type === 'add') {
      const library = await createLibrary(name, path)
      setLibraries((current) => [...current, library])
      setNotice(`Added “${library.name}”.`)
    } else if (editor?.type === 'rename') {
      const library = await renameLibrary(editor.library.id, name)
      updateLibrary(library)
      setNotice(`Renamed library to “${library.name}”.`)
    }
    closeEditor()
  }

  async function remove(library: Library) {
    setBusy(library.id)
    setRemoveError('')
    try {
      await removeLibrary(library.id)
      setLibraries((current) => current.filter((item) => item.id !== library.id))
      setNotice(`Removed “${library.name}”. Files were left untouched.`)
      closeEditor(true)
    } catch (error) {
      setRemoveError(error instanceof Error ? error.message : 'Could not remove the library. Try again.')
    } finally {
      setBusy(null)
    }
  }

  async function check(library: Library) {
    const controller = new AbortController()
    checkController.current = controller
    setBusy(library.id)
    setError('')
    setNotice(`Checking “${library.name}”…`)
    try {
      const result = await checkLibraryAvailability(library.id, controller.signal)
      updateLibrary(result.library)
      setNotice(result.completed
        ? `“${result.library.name}”: ${availabilityLabels[result.library.availability].toLowerCase()}.`
        : `Check requested for “${library.name}”; still waiting for a result. Refresh the list shortly.`)
    } catch (error) {
      if (!controller.signal.aborted) {
        setNotice('')
        setError(error instanceof Error ? error.message : 'Could not check availability. Try again.')
      }
    } finally {
      if (!controller.signal.aborted) setBusy(null)
    }
  }

  return (
    <section id="library" aria-labelledby="library-title">
      <div className="library-toolbar">
        <div className="page-heading">
          <p className="eyebrow">Your collection</p>
          <h1 id="library-title">Libraries</h1>
          <p>Manage the folders that hold your comics and manga.</p>
        </div>
        <div className="button-group">
          <button className="button" type="button" disabled={locked} onClick={refreshLibraries}>Refresh</button>
          <button ref={addButton} className="button button-primary" type="button" disabled={locked} onClick={(event) => openEditor({ type: 'add' }, event.currentTarget)}>Add library</button>
        </div>
      </div>

      <p className="library-notice" role="status">{loading ? 'Loading libraries…' : notice}</p>
      {error && (
        <div className="error-banner" role="alert">
          <p>{error}</p>
          <button className="button" type="button" disabled={locked} onClick={refreshLibraries}>Retry</button>
        </div>
      )}

      {editor?.type === 'add' && <LibraryForm onSave={save} onCancel={() => closeEditor()} />}

      {!loading && !error && libraries.length === 0 && !editor && (
        <div className="library-placeholder">
          <h2>Your collection starts here.</h2>
          <p>Add a library root to register your local comics and manga.</p>
          <button className="button button-primary" type="button" onClick={(event) => openEditor({ type: 'add' }, event.currentTarget)}>Add your first library</button>
        </div>
      )}

      {libraries.length > 0 && (
        <ul className="library-list" aria-label="Registered libraries" aria-busy={loading}>
          {[...libraries].sort((a, b) => a.name.localeCompare(b.name) || a.id.localeCompare(b.id)).map((library) => (
            <li className="library-card" key={library.id}>
              <div className="library-details">
                <h2>{library.name}</h2>
                <p className="library-path">{library.path}</p>
                <div className="library-metadata">
                  <span className={`availability availability-${library.availability}`}>{availabilityLabels[library.availability]}</span>
                  <span>{library.last_checked_at
                    ? <>Last checked <time dateTime={library.last_checked_at}>{new Date(library.last_checked_at).toLocaleString()}</time></>
                    : 'No availability check recorded'}</span>
                </div>
              </div>
              <div className="button-group" aria-label={`Actions for ${library.name}`}>
                <button className="button" type="button" disabled={locked} onClick={() => void check(library)}>{busy === library.id && !editor ? 'Checking…' : 'Check availability'}</button>
                <button className="button" type="button" disabled={locked} onClick={(event) => openEditor({ type: 'rename', library }, event.currentTarget)}>Rename</button>
                <button className="button" type="button" disabled={locked} onClick={(event) => openEditor({ type: 'remove', library }, event.currentTarget)}>Remove</button>
              </div>
              {editor?.type === 'rename' && editor.library.id === library.id && <LibraryForm library={library} onSave={save} onCancel={() => closeEditor()} />}
              {editor?.type === 'remove' && editor.library.id === library.id && (
                <div className="remove-confirmation" role="group" aria-label={`Remove ${library.name}`}>
                  <h2>Remove “{library.name}”?</h2>
                  <p>This removes the library registration. Files in the library root stay untouched.</p>
                  {removeError && <p className="error-message" role="alert">{removeError}</p>}
                  <div className="button-group">
                    <button autoFocus className="button" type="button" disabled={busy !== null} onClick={() => closeEditor()}>Cancel</button>
                    <button className="button button-primary" type="button" disabled={busy !== null} onClick={() => void remove(library)}>{busy ? 'Removing…' : 'Remove library'}</button>
                  </div>
                </div>
              )}
            </li>
          ))}
        </ul>
      )}
      <p className="availability-help">Availability indicates whether the library root is present as a directory. An unavailable root stays registered.</p>
    </section>
  )
}
