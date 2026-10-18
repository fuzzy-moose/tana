// @vitest-environment jsdom
import { cleanup, render as testingRender, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Theme } from '@radix-ui/themes'
import type { ReactNode } from 'react'

import { afterEach, expect, test, vi } from 'vitest'
import MetadataProxies from './MetadataProxies'
import type { MetadataProxyChannel, MetadataProxyInput, MetadataProxyStatus } from './metadataProxies'

const render = (ui: ReactNode) => testingRender(ui, { wrapper: Theme })
const page = () => <MetadataProxies available refreshKey={0} interval={5000} />

afterEach(() => { cleanup(); vi.unstubAllGlobals() })

const channel = (fields: Partial<MetadataProxyChannel> = {}): MetadataProxyChannel => ({
  id: 'one', name: 'Proxy A', proxy_url: 'http://proxy.example:8080', username: '', user_agent: 'Browser default',
  enabled: true, has_password: false, state: 'idle', batch_size: 0, ...fields,
})

// A fake collector keeping proxy state, as the real one would across requests.
function fakeCollector(initial: Partial<MetadataProxyStatus> = {}) {
  const state: MetadataProxyStatus = { enabled: false, auto_remove_inactive: false, channels: [], rate_interval_ms: 30000, default_user_agent: 'Browser default', ...initial }
  const fetchMock = vi.fn<typeof fetch>(async (input, init) => {
    const path = String(input)
    if (path === '/api/collector/metadata/proxies' && init?.method === 'PUT') Object.assign(state, JSON.parse(String(init.body)))
    else if (path.endsWith('/channels') && init?.method === 'POST') {
      const { password, ...fields } = JSON.parse(String(init.body)) as MetadataProxyInput
      state.channels = [...state.channels, channel({ ...fields, id: 'one', has_password: !!password })]
    } else if (path.endsWith('/channels/one') && init?.method === 'PUT') {
      const { password: _password, ...fields } = JSON.parse(String(init.body)) as MetadataProxyInput
      state.channels = state.channels.map((item) => item.id === 'one' ? { ...item, ...fields } : item)
    } else if (path.endsWith('/channels/one') && init?.method === 'DELETE') state.channels = []
    else if (path === '/api/collector/metadata/proxies/import' && init?.method === 'POST') {
      state.channels = [channel({ id: 'imported', name: 'Imported proxy', proxy_url: 'https://proxy.example:1080' })]
      return Response.json({ status: state, added: 1, duplicates: 2 })
    } else if (path !== '/api/collector/metadata/proxies') throw new Error(`Unexpected request: ${path}`)
    return Response.json(state)
  })
  vi.stubGlobal('fetch', fetchMock)
  return { state, fetchMock }
}

test('saves each proxy setting switch without touching the other', async () => {
  const { state } = fakeCollector()
  const user = userEvent.setup()
  render(page())
  const cleanupSwitch = await screen.findByRole('switch', { name: 'Remove proxies after 5 minutes without a successful request' })
  expect(screen.getByText('Up to 25 galleries per batch · one batch per channel every 30 s.')).toBeTruthy()
  await user.click(cleanupSwitch)
  await waitFor(() => expect(state.auto_remove_inactive).toBe(true))
  expect(state.enabled).toBe(false)
  await user.click(screen.getByRole('switch', { name: 'Proxy metadata collection' }))
  await waitFor(() => expect(state.enabled).toBe(true))
  expect(state.auto_remove_inactive).toBe(true)
})

test('adds and edits a channel through the dialog, keeping the write-only password on edit', async () => {
  const { fetchMock } = fakeCollector()
  const user = userEvent.setup()
  render(page())
  await screen.findByText(/No proxy channels configured/)
  await user.click(screen.getByRole('button', { name: 'Add channel' }))
  const dialog = screen.getByRole('dialog', { name: 'Add channel' })
  await user.type(within(dialog).getByLabelText('Channel name'), 'Proxy A')
  await user.type(within(dialog).getByLabelText(/^Proxy URL/), 'http://proxy.example:8080')
  await user.type(within(dialog).getByLabelText(/^Proxy password/), 'private-password')
  expect((within(dialog).getByLabelText('User-agent') as HTMLInputElement).value).toBe('Browser default')
  await user.click(within(dialog).getByRole('button', { name: 'Save channel' }))
  await screen.findByRole('rowheader', { name: /Proxy A/ })
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/metadata/proxies/channels', expect.objectContaining({ method: 'POST', body: expect.stringContaining('"password":"private-password"') }))

  await user.click(screen.getByRole('button', { name: 'Edit' }))
  const edit = screen.getByRole('dialog', { name: 'Edit channel' })
  expect((within(edit).getByLabelText('Channel name') as HTMLInputElement).value).toBe('Proxy A')
  await user.clear(within(edit).getByLabelText('Channel name'))
  await user.type(within(edit).getByLabelText('Channel name'), 'Proxy renamed')
  await user.click(within(edit).getByRole('button', { name: 'Save channel' }))
  await screen.findByRole('rowheader', { name: /Proxy renamed/ })
  const update = fetchMock.mock.calls.find(([path, init]) => String(path).endsWith('/channels/one') && init?.method === 'PUT')![1]!
  expect(JSON.parse(String(update.body))).not.toHaveProperty('password')
})

test('enables a channel from its row switch and removes it with the trash button', async () => {
  const { state } = fakeCollector({ enabled: true, channels: [channel({ enabled: false, state: 'disabled' })] })
  const user = userEvent.setup()
  render(page())
  await user.click(await screen.findByRole('switch', { name: 'Enable Proxy A' }))
  await waitFor(() => expect(state.channels[0].enabled).toBe(true))
  await user.click(screen.getByRole('button', { name: 'Remove Proxy A' }))
  await screen.findByText(/No proxy channels configured/)
})

test('maps channel states to labels and resets cooldowns', async () => {
  const until = '2026-10-18T15:10:00Z'
  const { fetchMock } = fakeCollector({
    enabled: true,
    channels: [
      channel({ id: 'a', name: 'Cooling', state: 'waiting_cooldown', ban_until: until }),
      channel({ id: 'b', name: 'Rejected', state: 'authentication_required', last_error: 'proxy_authentication_required' }),
      channel({ id: 'c', name: 'Busy', state: 'running', batch_size: 25 }),
    ],
  })
  const user = userEvent.setup()
  render(page())
  await screen.findByRole('rowheader', { name: /Cooling/ })
  expect(screen.getByText('Panda cooldown')).toBeTruthy()
  expect(screen.getByText('Credentials required')).toBeTruthy()
  expect(screen.getByText('25 galleries in active batch')).toBeTruthy()
  expect(screen.getByText('1 channel in Panda cooldown')).toBeTruthy()
  await user.click(screen.getByRole('button', { name: 'Reset cooldowns' }))
  await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/collector/bans/proxies/reset', expect.objectContaining({ method: 'POST' })))
})

test('imports pasted addresses with one shared type and reports the result', async () => {
  const { fetchMock } = fakeCollector()
  const user = userEvent.setup()
  render(page())
  await screen.findByText(/No proxy channels configured/)
  await user.click(screen.getByRole('button', { name: 'Paste proxy list' }))
  const dialog = screen.getByRole('dialog', { name: 'Paste proxy list' })
  expect(within(dialog).getByRole('combobox', { name: 'Proxy type' }).textContent).toBe('SOCKS5')
  await user.type(within(dialog).getByLabelText(/^Proxy addresses/), 'proxy.example:1080')
  await user.click(within(dialog).getByRole('combobox', { name: 'Proxy type' }))
  await user.click(screen.getByRole('option', { name: 'HTTPS' }))
  await user.click(within(dialog).getByRole('button', { name: 'Import channels' }))
  await screen.findByText('Added 1 channel. Skipped 2 duplicates.')
  expect(fetchMock).toHaveBeenCalledWith('/api/collector/metadata/proxies/import', expect.objectContaining({
    method: 'POST', body: JSON.stringify({ proxies: 'proxy.example:1080', protocol: 'https', enabled: true }),
  }))
  expect(screen.getByRole('rowheader', { name: /Imported proxy/ })).toBeTruthy()
  expect(screen.queryByRole('dialog')).toBeNull()
})

test('keeps the pasted list in the dialog after a rejected import', async () => {
  vi.stubGlobal('fetch', vi.fn<typeof fetch>(async (input, init) => {
    if (String(input).endsWith('/import') && init?.method === 'POST') return Response.json({ error: 'proxy_list_invalid' }, { status: 400 })
    return Response.json({ enabled: true, auto_remove_inactive: false, channels: [], rate_interval_ms: 30000, default_user_agent: 'Browser default' })
  }))
  const user = userEvent.setup()
  render(page())
  await screen.findByText(/No proxy channels configured/)
  await user.click(screen.getByRole('button', { name: 'Paste proxy list' }))
  const dialog = screen.getByRole('dialog')
  await user.type(within(dialog).getByLabelText(/^Proxy addresses/), 'https://proxy.example:1080')
  await user.click(within(dialog).getByRole('button', { name: 'Import channels' }))
  expect((await within(dialog).findByRole('alert')).textContent).toContain('one host:port per line, without a URL scheme')
  expect((within(dialog).getByLabelText(/^Proxy addresses/) as HTMLTextAreaElement).value).toBe('https://proxy.example:1080')
})
