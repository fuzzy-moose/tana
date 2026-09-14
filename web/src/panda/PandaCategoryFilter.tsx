import { Button, DropdownMenu } from '@radix-ui/themes'
import { MixerHorizontalIcon } from '@radix-ui/react-icons'
import { pandaCategories } from './categories'

export default function PandaCategoryFilter({ categories, onChange, label = 'Categories' }: {
  categories: string[]
  onChange: (categories: string[]) => void
  label?: string
}) {
  return <DropdownMenu.Root>
    <DropdownMenu.Trigger><Button variant="soft" color={categories.length ? undefined : 'gray'} size="2"><MixerHorizontalIcon />{label}{categories.length ? ` · ${categories.length}` : ''}</Button></DropdownMenu.Trigger>
    <DropdownMenu.Content>
      <DropdownMenu.Label>Panda gallery categories</DropdownMenu.Label>
      {pandaCategories.map((label) => {
        const category = label.toLowerCase()
        return <DropdownMenu.CheckboxItem key={category} checked={categories.includes(category)} onSelect={(event) => event.preventDefault()}
          onCheckedChange={(checked) => onChange(checked ? [...categories, category] : categories.filter((value) => value !== category))}>{label}</DropdownMenu.CheckboxItem>
      })}
      <DropdownMenu.Separator />
      <DropdownMenu.Item disabled={!categories.length} onSelect={() => onChange([])}>All categories</DropdownMenu.Item>
    </DropdownMenu.Content>
  </DropdownMenu.Root>
}
