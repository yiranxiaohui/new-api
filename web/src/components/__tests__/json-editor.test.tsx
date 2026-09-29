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
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, test, vi } from 'vitest'

import { JsonEditor } from '../json-editor'

describe('JsonEditor number values', () => {
  test('stores a numeric row value as a JSON number', () => {
    const onChange = vi.fn()
    render(
      <JsonEditor value='{"vip":1}' onChange={onChange} valueType='number' />
    )

    fireEvent.change(screen.getByDisplayValue('1'), {
      target: { value: '0.8' },
    })

    expect(JSON.parse(onChange.mock.lastCall?.[0])).toEqual({ vip: 0.8 })
  })

  test.each(['abc', ''])(
    'keeps non-numeric input %j as text instead of silently storing 0',
    (input) => {
      const onChange = vi.fn()
      render(
        <JsonEditor value='{"vip":1}' onChange={onChange} valueType='number' />
      )

      fireEvent.change(screen.getByDisplayValue('1'), {
        target: { value: input },
      })

      expect(JSON.parse(onChange.mock.lastCall?.[0])).toEqual({ vip: input })
    }
  )

  test('keeps an explicit zero as a number', async () => {
    const onChange = vi.fn()
    render(
      <JsonEditor value='{"vip":1}' onChange={onChange} valueType='number' />
    )

    const input = screen.getByDisplayValue('1')
    await userEvent.clear(input)
    await userEvent.type(input, '0')

    expect(JSON.parse(onChange.mock.lastCall?.[0])).toEqual({ vip: 0 })
  })
})
