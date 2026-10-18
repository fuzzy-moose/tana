import { createContext, useContext, useEffect } from 'react'
import type { ReactNode } from 'react'

/** Setter for the node the shell renders at the right of the page header. */
export const HeaderAsideContext = createContext<(node: ReactNode) => void>(() => {})

/** Shows `node` in the page header while the calling page is mounted. */
export function useHeaderAside(node: string | null) {
  const setAside = useContext(HeaderAsideContext)
  useEffect(() => {
    setAside(node)
    return () => setAside(null)
  }, [setAside, node])
}
