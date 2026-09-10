/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createRootRoute,
  createRouter,
  createMemoryHistory,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, expect, it, vi } from 'vitest'

import { SettingsPageProvider } from '@/features/system-settings/components/settings-page-context'
import { api } from '@/lib/api'

import { withdrawalConfigDigest } from '../api'
import { parseCNYCents } from '../types'
import { WithdrawalForm } from '../withdrawal-form'
import { WithdrawalSettings } from '../withdrawal-settings'

afterEach(() => vi.restoreAllMocks())
it.each([
  ['1.23', 123],
  ['0.01', 1],
  ['100', 10000],
  ['-1', null],
  ['1.001', null],
  ['1e2', null],
  ['', null],
])('parses CNY input %s into exact cents', (input, expected) => {
  expect(parseCNYCents(input as string)).toBe(expected)
})
it('requires a quote and sufficient referral quota before requesting security verification', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { quota: 10000, amount_cents: 100 } },
  })
  const post = vi.spyOn(api, 'post')
  const user = userEvent.setup()
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <WithdrawalForm
        availableQuota={5000}
        policy={{
          enabled: true,
          min_cents: 100,
          max_cents: 10000,
          cny_per_unit: '7',
        }}
        onSuccess={vi.fn()}
      />
    </QueryClientProvider>
  )
  await user.type(screen.getByLabelText('Withdrawal amount (CNY)'), '1')
  await user.type(screen.getByLabelText('Alipay account'), 'payee@example.com')
  await user.type(screen.getByLabelText('Recipient legal name'), 'Test Payee')
  await screen.findByText('Insufficient available referral rewards.')
  expect(
    screen.getByRole('button', { name: 'Request withdrawal' })
  ).toBeDisabled()
  expect(get).toHaveBeenCalled()
  expect(post).not.toHaveBeenCalled()
})

it('binds Unicode and HTML characters in payout settings to the server digest', async () => {
  const digest = await withdrawalConfigDigest({
    enabled: false,
    gateway: 'https://pay.yunnet.top',
    pid: '1001',
    api_key: '',
    notify_url: '',
    cny_per_unit: '7',
    min_cents: 100,
    max_cents: 10000,
    scene: '测试 & < >\u2028\u2029',
    scene_infos: [],
  })
  expect(digest).toBe(
    'ef7dbb1924fb8e17aa81938c214ad69e054d36efae9afd2c1de3d808188fc60e'
  )
})

it('keeps the same withdrawal details after a network failure and requires a new verification', async () => {
  const get = vi.spyOn(api, 'get').mockImplementation(async (url) => ({
    data: {
      success: true,
      data:
        url === '/api/verify/methods'
          ? {
              scope: 'withdrawal.create',
              methods: [{ method: '2fa', available: true }],
              oauth_providers: [],
              password_encryption_enabled: false,
            }
          : { quota: 100000, amount_cents: 140 },
    },
  }))
  let attempts = 0
  const post = vi.spyOn(api, 'post').mockImplementation(async (url) => {
    if (url === '/api/verify') {
      return {
        data: {
          success: true,
          data: {
            proof_token: 'test-proof',
            method: '2fa',
            scope: 'withdrawal.create',
            expires_at: Math.floor(Date.now() / 1000) + 300,
          },
        },
      }
    }
    attempts++
    if (attempts === 1) throw new Error('Network error')
    return { data: { success: true, data: { id: 'accepted' } } }
  })
  const user = userEvent.setup()
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const success = vi.fn()
  const busy = vi.fn()
  render(
    <QueryClientProvider client={client}>
      <WithdrawalForm
        availableQuota={500000}
        policy={{
          enabled: true,
          min_cents: 100,
          max_cents: 10000,
          cny_per_unit: '7',
        }}
        onSuccess={success}
        onBusyChange={busy}
      />
    </QueryClientProvider>
  )
  await user.type(screen.getByLabelText('Withdrawal amount (CNY)'), '1.40')
  await user.type(
    screen.getByLabelText('Alipay account'),
    'recipient@example.com'
  )
  await user.type(screen.getByLabelText('Recipient legal name'), 'Recipient')
  for (let attempt = 0; attempt < 2; attempt++) {
    await user.click(screen.getByRole('button', { name: 'Request withdrawal' }))
    await user.type(
      await screen.findByLabelText('Authenticator code or backup code'),
      '123456'
    )
    expect(screen.getByLabelText('Withdrawal amount (CNY)')).toBeDisabled()
    await user.click(screen.getByRole('button', { name: 'Verify' }))
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Request withdrawal' })
      ).toBeEnabled()
    )
  }
  const payouts = post.mock.calls.filter(
    ([url]) => url === '/api/user/withdrawals'
  )
  expect(payouts).toHaveLength(2)
  expect(payouts[0][1]).toEqual(payouts[1][1])
  expect(payouts[1][1]).toMatchObject({
    amount_cents: 140,
    quota: 100000,
    payee_account: 'recipient@example.com',
    payee_name: 'Recipient',
  })
  expect(success).toHaveBeenCalledOnce()
  expect(busy.mock.calls).toEqual([[true], [false], [true], [false]])
  expect(get).toHaveBeenCalledWith(
    '/api/verify/methods',
    expect.objectContaining({ params: { scope: 'withdrawal.create' } })
  )
})

function SettingsHarness() {
  const [actions, setActions] = useState<HTMLDivElement | null>(null)
  return (
    <>
      <div ref={setActions} />
      <SettingsPageProvider actionsContainer={actions}>
        <WithdrawalSettings />
      </SettingsPageProvider>
    </>
  )
}
it('clears the payout key after a verified settings save', async () => {
  const config = {
    enabled: false,
    gateway: 'https://pay.yunnet.top',
    pid: '1001',
    api_key: '',
    notify_url: 'https://example.com/api/user/withdrawals/notify',
    cny_per_unit: '7',
    min_cents: 100,
    max_cents: 10000,
    scene: 'Referral',
    scene_infos: [],
  }
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/verify/methods') {
      return {
        data: {
          success: true,
          data: {
            scope: 'withdrawal.configure',
            methods: [{ method: '2fa', available: true }],
            oauth_providers: [],
            password_encryption_enabled: false,
          },
        },
      }
    }
    if (url === '/api/user/withdrawals/admin/config') {
      return { data: { success: true, data: { config, key_configured: true } } }
    }
    return { data: { success: true, data: { items: [], has_more: false } } }
  })
  vi.spyOn(api, 'post').mockResolvedValue({
    data: {
      success: true,
      data: {
        proof_token: 'config-proof',
        scope: 'withdrawal.configure',
        method: '2fa',
        expires_at: Math.floor(Date.now() / 1000) + 300,
      },
    },
  })
  const put = vi
    .spyOn(api, 'put')
    .mockResolvedValue({ data: { success: true, data: {} } })
  const route = createRootRoute({ component: SettingsHarness })
  const router = createRouter({
    routeTree: route,
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const user = userEvent.setup()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  const key = await screen.findByLabelText('UniPay payout API key')
  await user.type(key, 'a'.repeat(64))
  await user.click(screen.getByRole('button', { name: 'Save Changes' }))
  await user.type(
    await screen.findByLabelText('Authenticator code or backup code'),
    '123456'
  )
  await user.click(screen.getByRole('button', { name: 'Verify' }))
  await waitFor(() => expect(key).toHaveValue(''))
  expect(put).toHaveBeenCalledWith(
    '/api/user/withdrawals/admin/config',
    expect.objectContaining({ api_key: 'a'.repeat(64) }),
    expect.objectContaining({ headers: { 'X-Security-Proof': 'config-proof' } })
  )
  expect(screen.getByRole('button', { name: 'Save Changes' })).toBeEnabled()
  await waitFor(() =>
    expect(client.getMutationCache().getAll()).toHaveLength(0)
  )
})
