import { readerLink } from '../navigation'
import GalleryImage from './GalleryImage'
import { useGallery } from './useGallery'
import './Galleries.css'

export default function GalleryDetail({ id }: { id: string }) {
  const { gallery, error, retry } = useGallery(id)
  return <section aria-labelledby="gallery-title">
    <a className="back-link" href="#/">← Galleries</a>
    {error && <div className="error-banner" role="alert"><p>{error}</p><button className="button" onClick={retry}>Retry</button></div>}
    {!gallery && !error && <p role="status">Loading gallery…</p>}
    {gallery && <>
      <div className="gallery-detail-heading">
        <div className="gallery-cover detail-cover">{gallery.page_count > 0 ? <GalleryImage id={id} page={1} alt="" /> : <span className="image-placeholder">No pages</span>}</div>
        <div className="page-heading">
          <p className="eyebrow">Gallery</p>
          <h1 id="gallery-title">{gallery.title}</h1>
          <p>{gallery.page_count} {gallery.page_count === 1 ? 'page' : 'pages'}</p>
          {gallery.page_count > 0 ? <a className="button button-primary" href={readerLink(id)}>Read gallery</a> : <p>This gallery has no pages to read.</p>}
        </div>
      </div>
      {gallery.page_count > 0 && <>
        <div className="gallery-meta"><h2>Pages</h2><span>Select a page to start reading</span></div>
        <ol className="page-grid" aria-label="Gallery pages">
          {Array.from({ length: gallery.page_count }, (_, index) => index + 1).map((page) => <li key={page}>
            <a className="page-thumbnail" href={readerLink(id, page)} aria-label={`Read from page ${page}`}>
              <div className="gallery-cover"><GalleryImage id={id} page={page} alt="" /></div>
              <span>{page}</span>
            </a>
          </li>)}
        </ol>
      </>}
    </>}
  </section>
}
