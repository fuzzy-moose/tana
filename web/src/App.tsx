import { lazy, Suspense } from 'react'
import { Button, Flex, Heading, Text, Theme } from '@radix-ui/themes'
import { ArchiveIcon, GlobeIcon, ImageIcon, MixerHorizontalIcon } from '@radix-ui/react-icons'
import './App.css'
import Galleries from './galleries/Galleries'
import GalleryDetail from './galleries/GalleryDetail'
import Reader from './galleries/Reader'
import Libraries from './libraries/Libraries'
import SourceCleanup from './libraries/SourceCleanup'
import LibraryRefresh from './libraries/LibraryRefresh'
import PandaCatalog from './panda/PandaCatalog'
import PandaLookupPage from './panda/PandaLookupPage'
import { useRoute } from './navigation'

const Collector = lazy(() => import('./collector/Collector'))

function App() {
  const route = useRoute()
  const catalog = route.kind === 'galleries' || route.kind === 'panda'
  const navigation = [
    { label: 'Galleries', href: '#/', icon: ImageIcon, active: route.kind === 'galleries' || route.kind === 'detail' },
    { label: 'Panda', href: '#/panda', icon: GlobeIcon, active: route.kind === 'panda' || route.kind === 'panda-lookup' },
    { label: 'Libraries', href: '#/libraries', icon: ArchiveIcon, active: route.kind === 'libraries' || route.kind === 'source-cleanup' || route.kind === 'library-refresh' },
    { label: 'Collector', href: '#/collector', icon: MixerHorizontalIcon, active: route.kind === 'collector' },
  ]

  return <Theme appearance="dark" accentColor="green" grayColor="sage" radius="medium" panelBackground="solid">
    {route.kind === 'reader'
      ? <Reader key={`${route.id}:${route.page}:${route.lastPage ?? ''}`} id={route.id} initialPage={route.page} initialLastPage={route.lastPage} />
      : <div className={`app-shell${catalog ? ' app-shell-catalog' : ''}`}>
        <a className="skip-link" href="#main-content" onClick={(event) => {
          event.preventDefault()
          document.getElementById('main-content')?.focus()
        }}>Skip to content</a>
        <aside className="sidebar">
          <a className="brand" href="#/" aria-label="Tana home">
            <img src="/favicon.svg" width="26" height="26" alt="" />
            <Text size="5" weight="bold">tana</Text>
          </a>
          <nav aria-label="Main navigation" className="main-navigation">
            {navigation.map(({ label, href, icon: Icon, active }) => <Button key={href} asChild variant={active ? 'soft' : 'ghost'} color={active ? undefined : 'gray'} className="main-nav-link">
              <a href={href} aria-current={active ? 'page' : undefined}><Icon width="18" height="18" />{label}</a>
            </Button>)}
          </nav>
        </aside>
        <main id="main-content" className="app-main" tabIndex={-1}>
          {route.kind === 'libraries' && <Libraries />}
          {route.kind === 'source-cleanup' && <SourceCleanup />}
          {route.kind === 'library-refresh' && <LibraryRefresh key={route.libraryID ?? 'all'} libraryID={route.libraryID} />}
          {route.kind === 'collector' && <Suspense fallback={<Text as="p" size="2" color="gray" role="status">Loading collector…</Text>}><Collector page={route.page} /></Suspense>}
          {route.kind === 'panda' && <PandaCatalog search={route.search} cursor={route.cursor} includeExpunged={route.includeExpunged} categories={route.categories} bypassDefault={route.bypassDefault} />}
          {route.kind === 'panda-lookup' && <PandaLookupPage />}
          {route.kind === 'galleries' && <Galleries search={route.search} page={route.page} categories={route.categories} sort={route.sort} />}
          {route.kind === 'detail' && <GalleryDetail key={route.id} id={route.id} />}
          {route.kind === 'not-found' && <Flex asChild direction="column" align="start" gap="4"><section>
            <Heading as="h1">Page not found</Heading>
            <Button asChild><a href="#/">Return to Galleries</a></Button>
          </section></Flex>}
        </main>
      </div>}
  </Theme>
}

export default App
