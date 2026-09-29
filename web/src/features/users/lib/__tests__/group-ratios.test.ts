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
import { describe, expect, test } from 'vitest'

import type { User } from '../../types'
import {
  isValidUserGroupRatios,
  transformFormDataToPayload,
  transformUserToFormDefaults,
  userFormSchema,
  USER_FORM_DEFAULT_VALUES,
} from '../user-form'

describe('per-user group ratio validation', () => {
  test.each([
    ['', true],
    ['  ', true],
    ['{}', true],
    ['{"vip":0.8,"default":0}', true],
    ['{"vip":100}', true],
    ['{"vip":-0.1}', false],
    ['{"vip":100.5}', false],
    ['{"vip":"0.8"}', false],
    ['{"vip":null}', false],
    ['{"":1}', false],
    ['{" vip":1}', false],
    ['[1]', false],
    ['{"vip":', false],
  ])(
    'accepts %j only when it maps group names to ratios in [0, 100]: %s',
    (raw, valid) => {
      expect(isValidUserGroupRatios(raw)).toBe(valid)
    }
  )

  test('reports an invalid group ratio on the group_ratios field', () => {
    const result = userFormSchema.safeParse({
      ...USER_FORM_DEFAULT_VALUES,
      username: 'alice',
      group_ratios: '{"vip":"abc"}',
    })
    expect(result.success).toBe(false)
    expect(result.error?.issues[0]).toMatchObject({
      path: ['group_ratios'],
      message: 'Each group ratio must be a number between 0 and 100',
    })
  })
})

describe('per-user group ratio payload', () => {
  test('sends an empty string so clearing the editor removes every override', () => {
    const payload = transformFormDataToPayload(
      { ...USER_FORM_DEFAULT_VALUES, username: 'alice', group_ratios: '' },
      7
    )
    expect(payload.group_ratios).toBe('')
  })

  test('sends the edited JSON for an update and omits it on create', () => {
    const values = {
      ...USER_FORM_DEFAULT_VALUES,
      username: 'alice',
      group_ratios: ' {"vip":0.8} ',
    }
    expect(transformFormDataToPayload(values, 7).group_ratios).toBe(
      '{"vip":0.8}'
    )
    expect(transformFormDataToPayload(values).group_ratios).toBeUndefined()
  })

  test('loads stored overrides into the form and treats a missing value as none', () => {
    const user = {
      id: 7,
      username: 'alice',
      display_name: 'alice',
      quota: 0,
      used_quota: 0,
      request_count: 0,
      group: 'default',
      status: 1,
      role: 1,
    } as User
    expect(
      transformUserToFormDefaults({ ...user, group_ratios: '{"vip":0.8}' })
        .group_ratios
    ).toBe('{"vip":0.8}')
    expect(transformUserToFormDefaults(user).group_ratios).toBe('')
  })
})
