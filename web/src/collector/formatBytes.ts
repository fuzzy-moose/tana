const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']

/** Formats a byte count in binary units with one decimal, e.g. `2.1 KiB`. */
export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes.toLocaleString()} B`
  let value = bytes
  let unit = 0
  while (unit < units.length - 1 && (value >= 1024 || value.toFixed(1) === '1024.0')) { value /= 1024; unit++ }
  return `${value.toFixed(1)} ${units[unit]}`
}
