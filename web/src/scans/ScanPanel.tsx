import { Badge, Box, Callout, Card, Flex, Grid, Heading, Progress, Text } from '@radix-ui/themes'
import type { Library } from '../libraries/api'
import type { ScanControls } from './useScan'

const phases = {
  idle: 'Ready to scan',
  discovering: 'Discovering sources…',
  importing: 'Importing sources…',
  completed: 'Scan complete',
  completed_with_errors: 'Scan complete with errors',
  failed: 'Scan failed',
}

export default function ScanPanel({ scan, libraries }: { scan: ScanControls, libraries: Library[] }) {
  const status = scan.status
  const scope = status?.library_id ? libraries.find((library) => library.id === status.library_id)?.name ?? 'Selected library' : 'All libraries'
  const active = status && status.phase !== 'idle'
  return <Card size="3" aria-label="Library scanning">
    {scan.error && <Callout.Root color="red" role="alert" mb="3"><Callout.Text>{scan.error}</Callout.Text></Callout.Root>}
    <Flex direction="column" gap="3" role="status" aria-live="polite">
      <Flex align="center" justify="between" gap="3" wrap="wrap">
        <Box>
          <Heading as="h2" size="3">Scan libraries</Heading>
          <Text as="p" size="1" color="gray" mt="1">Discover new sources and create galleries from their images.</Text>
        </Box>
        <Flex align="center" gap="2" wrap="wrap">
          <Badge color={status?.phase === 'failed' ? 'red' : status?.phase === 'completed_with_errors' ? 'amber' : 'gray'}>{status ? phases[status.phase] : 'Checking scan status…'}</Badge>
          {active && <Text size="1" color="gray">{scope}</Text>}
        </Flex>
      </Flex>
      {active && <>
        <Grid asChild columns={{ initial: '2', sm: '3', md: '6' }} gap="3" m="0">
          <dl>
            {[
              ['Discovered', status.discovered], ['Imported', status.imported], ['Galleries created', status.galleries_created],
              ['Failed sources', status.failed_sources], ['Discovery errors', status.discovery_errors], ['Skipped', status.skipped],
            ].map(([label, count]) => <Box key={label}>
              <Text asChild size="1" color="gray"><dt>{label}</dt></Text>
              <Text asChild size="4" weight="medium" m="0"><dd>{count}</dd></Text>
            </Box>)}
          </dl>
        </Grid>
        {status.phase === 'importing' && status.discovered > 0 && <Progress aria-label="Source import progress" value={(status.imported + status.failed_sources + status.skipped) / status.discovered * 100} />}
        {status.finished_at && <Text as="p" size="1" color="gray">Finished <time dateTime={status.finished_at}>{new Date(status.finished_at).toLocaleString()}</time></Text>}
      </>}
    </Flex>
  </Card>
}
