/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/
import { render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { InvoiceDetailDrawer } from '../components/invoice-detail-drawer'

afterEach(() => {
  vi.restoreAllMocks()
})

it('loads invoice details when opened by a controlled parent', async () => {
  const get = vi.spyOn(api, 'get').mockResolvedValue({
    data: {
      success: true,
      data: {
        id: 1,
        user_id: 2,
        username: 'user',
        invoice_no: 'INV-1',
        title_type: 1,
        title_name: '个人',
        tax_no: '',
        email: 'user@example.com',
        money: 500,
        status: 1,
        reject_reason: '',
        remark: '',
        create_time: 0,
        complete_time: 0,
        has_file: false,
        orders: [],
      },
    },
  })

  render(
    <InvoiceDetailDrawer
      invoiceId={1}
      open
      onOpenChange={vi.fn()}
      onChanged={vi.fn()}
    />
  )

  expect(await screen.findByText('INV-1')).toBeVisible()
  await waitFor(() => expect(get).toHaveBeenCalledWith('/api/invoice/1'))
})
