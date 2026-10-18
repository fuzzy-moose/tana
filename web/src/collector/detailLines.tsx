import type { ReactNode } from 'react'

/** One line per truthy entry for `StatusBadge`'s `detail`; undefined when there are none. */
export function detailLines(lines: (string | false | null | undefined)[]): ReactNode {
  const present = lines.filter((line): line is string => !!line)
  return present.length > 0 ? present.map((line) => <div key={line}>{line}</div>) : undefined
}
