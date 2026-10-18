import { Card, Flex, Heading, Inset, TabNav, Table, Text, VisuallyHidden } from '@radix-ui/themes'
import type { CSSProperties, ReactNode } from 'react'
import { collectorLink } from '../navigation'
import type { CollectorPage } from '../navigation'

export interface TableTab { state: string, label: string, count?: number, color?: 'red' | 'amber' | 'green' | 'gray' }

// `columns` lists widths (px numbers or CSS lengths such as '34%'); the single undefined entry is the flexible column.
// `actions` sit right-aligned in the header row, after `aside`.
// `empty` replaces the table (the tabs stay) when the current filter has no rows.
export default function TableCard({ title, titleId, caption, collectorPage, tabs, activeState = '', aside, actions, tabsLabel, columns, minWidth = 860, footnote, empty, children }: {
  title?: string, titleId?: string, caption: string, collectorPage: CollectorPage, tabs?: TableTab[], activeState?: string, aside?: string, actions?: ReactNode, tabsLabel?: string,
  columns: (number | string | undefined)[], minWidth?: number, footnote?: ReactNode, empty?: ReactNode, children?: ReactNode,
}) {
  const hasHeader = !!(title || tabs || aside || actions)
  return <Card>
    <Inset side="all" clip="padding-box">
      {hasHeader && <div className={tabs ? 'collector-table-card-header collector-table-card-header-tabs' : 'collector-table-card-header'}>
        {title && <Heading as="h2" size="4" id={titleId}>{title}</Heading>}
        {tabs && <TabNav.Root aria-label={tabsLabel ?? `Filter ${(title ?? caption).toLowerCase()}`}>
          {tabs.map((tab) => <TabNav.Link asChild active={tab.state === activeState} key={tab.state}>
            <a href={collectorLink(collectorPage, { state: tab.state })}>
              <Text color={tab.color}>{tab.label}</Text>{tab.count !== undefined && <Text color="gray" ml="1">{tab.count}</Text>}
            </a>
          </TabNav.Link>)}
        </TabNav.Root>}
        {aside && <Text size="1" color="gray" ml="auto">{aside}</Text>}
        {actions && <Flex gap="2" ml={aside ? undefined : 'auto'}>{actions}</Flex>}
      </div>}
      {empty ? <div className="collector-table-card-footnote">{empty}</div> : <div className="collector-table-scroll">
        <Table.Root size="2" layout="fixed" style={{ minWidth, '--table-min-width': `${minWidth}px` } as CSSProperties}>
          <caption><VisuallyHidden>{caption}</VisuallyHidden></caption>
          <colgroup>{columns.map((width, index) => <col key={index} style={width === undefined ? undefined : { width }} />)}</colgroup>
          {children}
        </Table.Root>
      </div>}
      {footnote && <Text as="p" size="1" color="gray" className="collector-table-card-footnote">{footnote}</Text>}
    </Inset>
  </Card>
}
