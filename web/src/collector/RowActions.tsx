import { TrashIcon } from '@radix-ui/react-icons'
import { Flex, IconButton } from '@radix-ui/themes'
import type { ReactNode } from 'react'

// Slots keep their widths when empty so buttons line up across rows.
// `slot2Width` and `deleteSlot` are table-level: pass the same values in every row.
export default function RowActions({ slot1, slot1Width, slot2, slot2Width, deleteSlot, onDelete, deleteLabel, deleteDisabled }: {
  slot1?: ReactNode, slot1Width: number, slot2?: ReactNode, slot2Width?: number,
  deleteSlot?: boolean, onDelete?: () => void, deleteLabel?: string, deleteDisabled?: boolean,
}) {
  return <Flex gap="2" justify="end">
    <div style={{ width: slot1Width }}>{slot1}</div>
    {slot2Width !== undefined && <div style={{ width: slot2Width }}>{slot2}</div>}
    {onDelete
      ? <IconButton color="red" variant="soft" aria-label={deleteLabel} disabled={deleteDisabled} onClick={onDelete}><TrashIcon /></IconButton>
      : deleteSlot && <div style={{ width: 'var(--space-6)' }} />}
  </Flex>
}
