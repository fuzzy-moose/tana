import { Flex, Heading, Link, Text } from '@radix-ui/themes'
import { ArrowLeftIcon } from '@radix-ui/react-icons'
import PandaLookup from './PandaLookup'
import './PandaCatalog.css'

export default function PandaLookupPage() {
  return <Flex asChild direction="column" gap="5"><section aria-labelledby="panda-lookup-page-title">
    <Flex direction="column" gap="3">
      <Link href="#/panda" size="2"><ArrowLeftIcon /> Panda catalog</Link>
      <Heading as="h1" size="6" id="panda-lookup-page-title">Gallery lookup</Heading>
      <Text as="p" size="2" color="gray">Find collected metadata by Panda gallery ID or URL. A URL can request metadata for a gallery the collector has not seen yet.</Text>
    </Flex>
    <PandaLookup />
  </section></Flex>
}
