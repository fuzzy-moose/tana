import { useEffect, useRef, useState } from 'react'
import { changeReferenceImport, listReferenceImports, referenceImportPageSize, uploadReferenceImport } from './referenceImports'
import type { ReferenceImport } from './referenceImports'

export function useReferenceImports(available: boolean, refreshKey: number) {
  const [snapshot, setSnapshot] = useState<{ imports: ReferenceImport[], offset: number } | null>(null)
  const [offset, setOffset] = useState(0)
  const [checking, setChecking] = useState(true)
  const [pending, setPending] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [transfer, setTransfer] = useState<{ filename: string, bytes: number, size: number } | null>(null)
  const [uploadErrors, setUploadErrors] = useState<string[]>([])
  const [error, setError] = useState('')
  const [requestError, setRequestError] = useState('')
  const [notice, setNotice] = useState('')
  const [revision, setRevision] = useState(0)
  const mutation = useRef<AbortController | null>(null)
  const upload = useRef<AbortController | null>(null)

  useEffect(() => () => { mutation.current?.abort(); upload.current?.abort() }, [])
  useEffect(() => {
    if (!available || pending) return
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout>
    async function poll() {
      setChecking(true)
      try {
        const result = await listReferenceImports(offset, controller.signal)
        if (controller.signal.aborted) return
        if (!Array.isArray(result.imports)) throw new Error('The collector returned an unexpected import history.')
        if (result.imports.length === 0 && offset > 0) {
          setOffset((value) => Math.max(0, value - referenceImportPageSize))
          return
        }
        setSnapshot({ imports: result.imports, offset })
        setError('')
      } catch (error) {
        if (!controller.signal.aborted) setError((error as Error).message)
      } finally {
        if (!controller.signal.aborted) {
          setChecking(false)
          timer = setTimeout(poll, 5000)
        }
      }
    }
    void poll()
    return () => { controller.abort(); clearTimeout(timer) }
  }, [available, pending, offset, revision, refreshKey])

  async function submit(files: File[]) {
    if (upload.current || !available || files.length === 0) return
    const controller = new AbortController()
    upload.current = controller
    setUploading(true)
    setUploadErrors([])
    setNotice('')
    let accepted = 0
    try {
      for (const file of files) {
        if (controller.signal.aborted) break
        setTransfer({ filename: file.name, bytes: 0, size: file.size })
        try {
          await uploadReferenceImport(file, controller.signal, (bytes) => {
            if (!controller.signal.aborted) setTransfer({ filename: file.name, bytes, size: file.size })
          })
          if (controller.signal.aborted) break
          accepted++
          setNotice(`${accepted} ${accepted === 1 ? 'file accepted' : 'files accepted'}. Imports continue on the collector.`)
          setOffset(0)
          setRevision((value) => value + 1)
        } catch (error) {
          if (controller.signal.aborted) break
          setUploadErrors((errors) => [...errors, `${file.name}: ${(error as Error).message}`])
        }
      }
    } finally {
      upload.current = null
      if (!controller.signal.aborted) {
        setTransfer(null)
        setUploading(false)
        setRevision((value) => value + 1)
      }
    }
  }

  async function change(id: string, action: 'cancel' | 'retry') {
    if (mutation.current || !available) return
    const controller = new AbortController()
    mutation.current = controller
    setPending(true)
    setRequestError('')
    try {
      const result = await changeReferenceImport(id, action, controller.signal)
      if (!controller.signal.aborted) setSnapshot((current) => current ? {
        ...current, imports: current.imports.map((item) => item.id === result.id ? result : item),
      } : current)
    } catch (error) {
      if (!controller.signal.aborted) setRequestError((error as Error).message)
    } finally {
      mutation.current = null
      if (!controller.signal.aborted) {
        setPending(false)
        setRevision((value) => value + 1)
      }
    }
  }

  const current = snapshot?.offset === offset ? snapshot : null
  return {
    imports: current?.imports.slice(0, referenceImportPageSize) ?? [],
    hasNext: (current?.imports.length ?? 0) > referenceImportPageSize,
    loaded: !!current, offset, checking, pending, uploading, transfer, uploadErrors, error, requestError, notice,
    stale: !!snapshot && (!available || !!error),
    refresh: () => setRevision((value) => value + 1),
    previous: () => setOffset((value) => Math.max(0, value - referenceImportPageSize)),
    next: () => setOffset((value) => value + referenceImportPageSize),
    submit, change,
  }
}
