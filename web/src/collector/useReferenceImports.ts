import { useEffect, useRef, useState } from 'react'
import { collectorLink } from '../navigation'
import { changeReferenceImport, listReferenceImports, referenceImportPageSize, uploadReferenceImport } from './referenceImports'
import { useMutation } from './useMutation'
import type { ReferenceImport, ReferenceImportAction } from './referenceImports'

// The previous response stays until the next arrives, so the pagination keeps its total across page changes.
// A page past the end is corrected by the page component once the total is known.
export function useReferenceImports(available: boolean, refreshKey: number, interval: number, page: number) {
  const [snapshot, setSnapshot] = useState<{ imports: ReferenceImport[], total: number } | null>(null)
  const offset = (page - 1) * referenceImportPageSize
  const [checking, setChecking] = useState(true)
  const [uploading, setUploading] = useState(false)
  const [transfer, setTransfer] = useState<{ filename: string, bytes: number, size: number } | null>(null)
  const [uploadErrors, setUploadErrors] = useState<string[]>([])
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [revision, setRevision] = useState(0)
  const mutation = useMutation()
  const { pending } = mutation
  const upload = useRef<AbortController | null>(null)

  useEffect(() => () => upload.current?.abort(), [])
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
        setSnapshot({ imports: result.imports, total: result.total })
        setError('')
      } catch (error) {
        if (!controller.signal.aborted) setError((error as Error).message)
      } finally {
        if (!controller.signal.aborted) {
          setChecking(false)
          timer = setTimeout(poll, interval)
        }
      }
    }
    void poll()
    return () => { controller.abort(); clearTimeout(timer) }
  }, [available, pending, offset, revision, refreshKey, interval])

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
          // New imports list first.
          if (page !== 1) window.location.hash = collectorLink('imports')
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

  async function change(id: string, action: ReferenceImportAction) {
    if (!available) return
    await mutation.run((signal) => changeReferenceImport(id, action, signal), {
      onSuccess: (result) => setSnapshot((current) => current ? {
        ...current, imports: current.imports.map((item) => item.id === result.id ? result : item),
      } : current),
      onSettled: () => setRevision((value) => value + 1),
    })
  }

  return {
    imports: snapshot?.imports ?? [], total: snapshot?.total ?? 0,
    loaded: !!snapshot, checking, pending, uploading, transfer, uploadErrors, error, requestError: mutation.error, notice,
    stale: !!snapshot && (!available || !!error),
    submit, change,
  }
}
