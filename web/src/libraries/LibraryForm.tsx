import { useId, useState } from 'react'
import type { FormEvent } from 'react'
import type { Library } from './api'

interface LibraryFormProps {
  library?: Library
  onSave: (name: string, path: string) => Promise<void>
  onCancel: () => void
}

export default function LibraryForm({ library, onSave, onCancel }: LibraryFormProps) {
  const [name, setName] = useState(library?.name ?? '')
  const [path, setPath] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const pathId = useId()
  const title = library ? 'Rename library' : 'Add library'

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (saving) return
    if (!name.trim()) {
      setError('Enter a library name.')
      return
    }
    setSaving(true)
    setError('')
    try {
      await onSave(name.trim(), path)
    } catch (error) {
      setError(error instanceof Error ? error.message : 'Could not save the library. Try again.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <form className="library-form" aria-label={title} onSubmit={submit}>
      <h2>{title}</h2>
      <fieldset className="form-fields" disabled={saving}>
        <label className="form-field">
          Name
          <input className="text-input" autoFocus name="name" autoComplete="off" required value={name} onChange={(event) => setName(event.target.value)} />
        </label>
        {!library && (
          <div className="form-field">
            <label htmlFor={pathId}>Library root</label>
            <input className="text-input" id={pathId} name="path" autoComplete="off" spellCheck={false} required value={path} onChange={(event) => setPath(event.target.value)} aria-describedby={`${pathId}-help`} placeholder="/mnt/comics" />
            <span id={`${pathId}-help`} className="field-help">Absolute directory path on the machine running Tana.</span>
          </div>
        )}
        {library && <p className="library-path">{library.path}</p>}
        {error && <p className="error-message" role="alert">{error}</p>}
        <div className="button-group">
          <button className="button button-primary" type="submit">{saving ? 'Saving…' : library ? 'Save name' : 'Add library'}</button>
          <button className="button" type="button" onClick={onCancel}>Cancel</button>
        </div>
      </fieldset>
    </form>
  )
}
