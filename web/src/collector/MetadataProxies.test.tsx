// @vitest-environment jsdom
import { cleanup, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'
import App from '../App'
import MetadataProxies from './MetadataProxies'
import type { MetadataProxyInput, MetadataProxyStatus } from './metadataProxies'

afterEach(() => { cleanup(); vi.unstubAllGlobals(); window.history.replaceState(null, '', '/') })

test('configures shared proxy channels, retaining write-only passwords on edit', async () => {
  const state: MetadataProxyStatus = { enabled: false, channels: [], rate_interval_ms: 2500, default_user_agent: 'Browser default' }
  const fetchMock = vi.fn<typeof fetch>(async (input, init) => {
    const path = String(input)
    if (path === '/api/collector/status') return Response.json({ configured: true, reachable: true, authenticated: true, status: { available: true } })
    if (path === '/api/collector/metadata/proxies' && init?.method === 'PUT') state.enabled = JSON.parse(String(init.body)).enabled
    else if (path.endsWith('/channels') && init?.method === 'POST') {
      const { password, ...fields } = JSON.parse(String(init.body)) as MetadataProxyInput
      state.channels = [{ ...fields, id: 'one', has_password: !!password, state: 'disabled', batch_size: 0 }]
    } else if (path.endsWith('/channels/one') && init?.method === 'PUT') {
      const { password, ...fields } = JSON.parse(String(init.body)) as MetadataProxyInput
      state.channels[0] = { ...state.channels[0], ...fields, has_password: password === undefined ? state.channels[0].has_password : !!password }
    } else if (path.endsWith('/channels/one') && init?.method === 'DELETE') state.channels = []
    else if (path !== '/api/collector/metadata/proxies') throw new Error(`Unexpected request: ${path}`)
    return Response.json(state)
  })
  vi.stubGlobal('fetch', fetchMock)
  window.history.replaceState(null, '', '/#/collector/proxies')
  const user = userEvent.setup()
  render(<App />)
  await screen.findByText(/No proxy channels configured/)
  await user.click(screen.getByRole('button', { name: 'Add channel' }))
  await user.type(screen.getByLabelText('Channel name'), 'Proxy A')
  await user.type(screen.getByLabelText(/^Proxy URL/), 'http://proxy.example:8080')
  await user.type(screen.getByLabelText('Proxy username'), 'user')
  await user.type(screen.getByLabelText(/^Proxy password/), 'private-password')
  expect((screen.getByLabelText('User-agent') as HTMLInputElement).value).toBe('Browser default')
  await user.click(screen.getByRole('button', { name: 'Save channel' }))
  await screen.findByRole('article', { name: 'Proxy A' })
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/metadata/proxies/channels', expect.objectContaining({ method: 'POST', body: expect.stringContaining('"password":"private-password"') }))
  await user.click(screen.getByRole('checkbox', { name: 'Enable proxy metadata collection' }))
  await waitFor(() => expect(state.enabled).toBe(true))
  await user.click(within(screen.getByRole('article', { name: 'Proxy A' })).getByRole('button', { name: 'Edit' }))
  expect((screen.getByLabelText(/^Proxy password/) as HTMLInputElement).value).toBe('')
  await user.clear(screen.getByLabelText('Channel name'))
  await user.type(screen.getByLabelText('Channel name'), 'Proxy renamed')
  await user.click(screen.getByRole('button', { name: 'Save channel' }))
  const article = await screen.findByRole('article', { name: 'Proxy renamed' })
  const update = fetchMock.mock.calls.find(([path, init]) => String(path).endsWith('/channels/one') && init?.method === 'PUT')![1]!
  expect(JSON.parse(String(update.body))).not.toHaveProperty('password')
  expect(state.channels[0].has_password).toBe(true)
  await user.click(within(article).getByRole('checkbox', { name: 'Enable Proxy renamed' }))
  await waitFor(() => expect(state.channels[0].enabled).toBe(false))
  await user.click(within(article).getByRole('button', { name: 'Remove' }))
  await screen.findByText(/No proxy channels configured/)
})

test('imports pasted addresses with one shared type without enabling global collection', async () => {
  const state: MetadataProxyStatus = { enabled: false, channels: [], rate_interval_ms: 2500, default_user_agent: 'Browser default' }
  const fetchMock = vi.fn<typeof fetch>(async (input, init) => {
    if (String(input) === '/api/collector/metadata/proxies/import' && init?.method === 'POST') {
      state.channels = [{ id: 'imported', name: 'Imported proxy', proxy_url: 'https://proxy.example:1080', username: '', user_agent: state.default_user_agent, enabled: true, has_password: false, state: 'disabled', batch_size: 0 }]
      return Response.json({ status: state, added: 1, duplicates: 2 })
    }
    if (String(input) === '/api/collector/metadata/proxies') return Response.json(state)
    throw new Error(`Unexpected request: ${String(input)}`)
  })
  vi.stubGlobal('fetch', fetchMock)
  const user = userEvent.setup()
  render(<MetadataProxies available refreshKey={0} />)
  await screen.findByText(/No proxy channels configured/)
  await user.click(screen.getByRole('button', { name: 'Paste proxy list' }))
  expect((screen.getByLabelText(/^Proxy type/) as HTMLSelectElement).value).toBe('socks5')
  await user.type(screen.getByLabelText(/^Proxy addresses/), 'proxy.example:1080\nproxy.example:1080\nproxy.example:1080')
  await user.selectOptions(screen.getByLabelText(/^Proxy type/), 'https')
  expect((screen.getByLabelText('Enable imported channels') as HTMLInputElement).checked).toBe(true)
  await user.click(screen.getByRole('button', { name: 'Import channels' }))
  await screen.findByText('Added 1 channel. Skipped 2 duplicates.')
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/metadata/proxies/import', expect.objectContaining({
    method: 'POST', body: JSON.stringify({ proxies: 'proxy.example:1080\nproxy.example:1080\nproxy.example:1080', protocol: 'https', enabled: true }),
  }))
  expect(screen.getByRole('article', { name: 'Imported proxy' })).toBeTruthy()
  expect((screen.getByLabelText('Enable proxy metadata collection') as HTMLInputElement).checked).toBe(false)
  expect(screen.queryByRole('button', { name: 'Import channels' })).toBeNull()
})

test('keeps pasted addresses and type available after a rejected list', async () => {
  const state: MetadataProxyStatus = { enabled: true, channels: [], rate_interval_ms: 2500, default_user_agent: 'Browser default' }
  const fetchMock = vi.fn<typeof fetch>(async (input, init) => {
    if (String(input) === '/api/collector/metadata/proxies/import' && init?.method === 'POST') return Response.json({ error: 'proxy_list_invalid' }, { status: 400 })
    if (String(input) === '/api/collector/metadata/proxies') return Response.json(state)
    throw new Error(`Unexpected request: ${String(input)}`)
  })
  vi.stubGlobal('fetch', fetchMock)
  const user = userEvent.setup()
  render(<MetadataProxies available refreshKey={0} />)
  await screen.findByText(/No proxy channels configured/)
  await user.click(screen.getByRole('button', { name: 'Paste proxy list' }))
  await user.type(screen.getByLabelText(/^Proxy addresses/), 'https://proxy.example:1080')
  await user.selectOptions(screen.getByLabelText(/^Proxy type/), 'https')
  await user.click(screen.getByLabelText('Enable imported channels'))
  await user.click(screen.getByRole('button', { name: 'Import channels' }))
  expect((await screen.findByRole('alert')).textContent).toContain('one host:port per line, without a URL scheme')
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/metadata/proxies/import', expect.objectContaining({
    method: 'POST', body: JSON.stringify({ proxies: 'https://proxy.example:1080', protocol: 'https', enabled: false }),
  }))
  expect((screen.getByLabelText(/^Proxy addresses/) as HTMLTextAreaElement).value).toBe('https://proxy.example:1080')
  expect((screen.getByLabelText(/^Proxy type/) as HTMLSelectElement).value).toBe('https')
  expect((screen.getByRole('button', { name: 'Import channels' }) as HTMLButtonElement).disabled).toBe(false)
  expect(screen.queryByRole('article')).toBeNull()
})
