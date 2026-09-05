import './App.css'
import Libraries from './libraries/Libraries'

function LibraryIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true">
      <path d="M3 4h4v16H3zM7 6h4v14H7zM14 4l4-1 4 16-4 1z" />
    </svg>
  )
}

function App() {
  return (
    <div className="app-shell">
      <a className="skip-link" href="#main-content">Skip to content</a>

      <header className="app-header">
        <a className="brand" href="#library" aria-label="Tana home">
          <img src="/favicon.svg" width="28" height="28" alt="" />
          <span>tana</span>
        </a>
        <span className="header-caption">Your personal reading space</span>
      </header>

      <aside className="sidebar">
        <nav aria-label="Main navigation">
          <p className="nav-label">Workspace</p>
          <a className="nav-link" href="#library" aria-current="page">
            <LibraryIcon />
            Libraries
          </a>
        </nav>
        <p className="sidebar-caption">A home for your comics &amp; manga.</p>
      </aside>

      <main id="main-content" tabIndex={-1}>
        <Libraries />
      </main>
    </div>
  )
}

export default App
