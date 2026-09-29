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

describe('JsonEditor key options', () => {
  test('picks a key from the dropdown instead of a free-text input', async () => {
    const onChange = vi.fn()
    const user = userEvent.setup()
    render(
      <JsonEditor
        value='{"":1}'
        onChange={onChange}
        valueType='number'
        keyLabel='Group'
        keyOptions={['default', 'vip']}
      />
    )

    await user.click(screen.getByRole('combobox', { name: 'Group' }))
    await user.click(screen.getByRole('option', { name: 'vip' }))

    expect(JSON.parse(onChange.mock.lastCall?.[0])).toEqual({ vip: 1 })
  })

  test('does not offer a key already used by another row', async () => {
    const user = userEvent.setup()
    render(
      <JsonEditor
        value='{"vip":0.8,"":1}'
        onChange={vi.fn()}
        keyLabel='Group'
        keyOptions={['default', 'vip']}
      />
    )

    const [, emptyRow] = screen.getAllByRole('combobox', { name: 'Group' })
    await user.click(emptyRow)

    expect(screen.getByRole('option', { name: 'default' })).toBeVisible()
    expect(
      screen.queryByRole('option', { name: 'vip' })
    ).not.toBeInTheDocument()
  })

  test('keeps showing a stored key that is no longer an allowed option', () => {
    render(
      <JsonEditor
        value='{"removed-group":0.5}'
        onChange={vi.fn()}
        keyLabel='Group'
        keyOptions={['default']}
      />
    )

    expect(screen.getByRole('combobox', { name: 'Group' })).toHaveValue(
      'removed-group'
    )
  })
})
