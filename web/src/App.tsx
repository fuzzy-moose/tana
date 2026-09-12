import './App.css'
import Galleries from './galleries/Galleries'
import GalleryDetail from './galleries/GalleryDetail'
import Reader from './galleries/Reader'
import Libraries from './libraries/Libraries'
import Collector from './collector/Collector'
import PandaCatalog from './panda/PandaCatalog'
import { useRoute } from './navigation'

function App() {
  const route = useRoute()
  if (route.kind === 'reader') return <Reader key={`${route.id}:${route.page}:${route.lastPage ?? ''}`} id={route.id} initialPage={route.page} initialLastPage={route.lastPage} />
  return (
    <div className={`app-shell${route.kind === 'galleries' || route.kind === 'panda' ? ' app-shell-galleries' : ''}`}>
      <a className="skip-link" href="#main-content" onClick={(event) => { event.preventDefault(); document.getElementById('main-content')?.focus() }}>Skip to content</a>
      <header className="app-header">
        <a className="brand" href="#/" aria-label="Tana home">
          <img src="/favicon.svg" width="28" height="28" alt="" /><span>tana</span>
        </a>
        <span className="header-caption">Your personal reading space</span>
      </header>
      <aside className="sidebar">
        <nav aria-label="Main navigation">
          <p className="nav-label">Workspace</p>
          <a className="nav-link" href="#/" aria-current={route.kind === 'galleries' || route.kind === 'detail' ? 'page' : undefined}>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true"><rect x="3" y="3" width="7" height="8" rx="1" /><rect x="14" y="3" width="7" height="8" rx="1" /><rect x="3" y="15" width="7" height="6" rx="1" /><rect x="14" y="15" width="7" height="6" rx="1" /></svg>
            Galleries
          </a>
          <a className="nav-link" href="#/libraries" aria-current={route.kind === 'libraries' ? 'page' : undefined}>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true"><path d="M3 4h4v16H3zM7 6h4v14H7zM14 4l4-1 4 16-4 1z" /></svg>
            Libraries
          </a>
          <a className="nav-link" href="#/panda" aria-current={route.kind === 'panda' ? 'page' : undefined}>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true"><circle cx="12" cy="12" r="9" /><path d="M3 12h18M12 3c5 5 5 13 0 18-5-5-5-13 0-18Z" /></svg>
            Panda
          </a>
          <a className="nav-link" href="#/collector" aria-current={route.kind === 'collector' ? 'page' : undefined}>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true"><rect x="4" y="3" width="16" height="7" rx="2" /><rect x="4" y="14" width="16" height="7" rx="2" /><path d="M8 6.5h2M8 17.5h2M12 10v4" /></svg>
            Collector
          </a>
        </nav>
        <p className="sidebar-caption">A home for your comics &amp; manga.</p>
      </aside>
      <main id="main-content" tabIndex={-1}>
        {route.kind === 'libraries' && <Libraries />}
        {route.kind === 'collector' && <Collector page={route.page} />}
        {route.kind === 'panda' && <PandaCatalog search={route.search} page={route.page} includeExpunged={route.includeExpunged} />}
        {route.kind === 'galleries' && <Galleries key={route.search} search={route.search} page={route.page} />}
        {route.kind === 'detail' && <GalleryDetail key={route.id} id={route.id} />}
        {route.kind === 'not-found' && <section><h1>Page not found</h1><a className="button" href="#/">Return to Galleries</a></section>}
      </main>
    </div>
  )
}

export default App
