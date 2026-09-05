// @vitest-environment jsdom
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, expect, test } from 'vitest'
import Pagination from './Pagination'

afterEach(cleanup)

const pageHref = (page: number) => `/items/${page}`

test('renders the page window with a current page, inert ellipses and caller-supplied links', () => {
  render(<Pagination page={10} totalPages={20} pageHref={pageHref} label="Item pages" />)

  expect(screen.getByRole('navigation', { name: 'Item pages' }).textContent).toBe('«1…78910111213…20»')
  const current = screen.getByLabelText('Page 10')
  expect(current.getAttribute('aria-current')).toBe('page')
  expect(current.hasAttribute('href')).toBe(false)
  expect(screen.getAllByText('…').every((item) => !item.hasAttribute('href'))).toBe(true)
  expect(screen.getByRole('link', { name: 'Page 1' }).getAttribute('href')).toBe('/items/1')
  expect(screen.getByRole('link', { name: 'Page 20' }).getAttribute('href')).toBe('/items/20')
  expect(screen.getByRole('link', { name: 'Previous' }).getAttribute('href')).toBe('/items/9')
  expect(screen.getByRole('link', { name: 'Next' }).getAttribute('href')).toBe('/items/11')
})

test('disables previous and next at the ends', () => {
  const { rerender } = render(<Pagination page={1} totalPages={2} pageHref={pageHref} />)
  expect(screen.getByRole<HTMLButtonElement>('button', { name: 'Previous' }).disabled).toBe(true)
  expect(screen.getByRole('link', { name: 'Next' }).getAttribute('href')).toBe('/items/2')

  rerender(<Pagination page={2} totalPages={2} pageHref={pageHref} />)
  expect(screen.getByRole<HTMLButtonElement>('button', { name: 'Next' }).disabled).toBe(true)
  expect(screen.getByRole('link', { name: 'Previous' }).getAttribute('href')).toBe('/items/1')
  expect(screen.getByRole('link', { name: 'Page 1' }).getAttribute('href')).toBe('/items/1')
})

test('uses the requested adjacent-page count', () => {
  render(<Pagination page={10} totalPages={20} adjacentCount={1} pageHref={pageHref} />)
  expect(screen.getByRole('navigation', { name: 'Pagination' }).textContent).toBe('«1…91011…20»')
})

test.each([0, 1])('hides pagination for %i total pages', (totalPages) => {
  render(<Pagination page={1} totalPages={totalPages} pageHref={pageHref} />)
  expect(screen.queryByRole('navigation')).toBeNull()
})
