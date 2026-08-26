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
import { render, screen } from '@testing-library/react'
import { describe, expect, test, vi } from 'vitest'

import { LegalConsent } from '../legal-consent'

describe('LegalConsent', () => {
  test('shows the default legal documents without custom legal settings', () => {
    render(
      <LegalConsent status={null} checked={false} onCheckedChange={vi.fn()} />
    )

    expect(screen.getByRole('checkbox')).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'Terms of Service' })
    ).toHaveAttribute('href', 'https://www.xtokenmirror.com/legal/terms')
    expect(
      screen.getByRole('link', { name: 'Usage Policy' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'Supported Regions' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'Service-Specific Terms' })
    ).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'User Agreement' })).toBeNull()
    expect(screen.queryByRole('link', { name: 'Privacy Policy' })).toBeNull()
  })

  test('appends configured custom legal documents', () => {
    render(
      <LegalConsent
        status={{
          user_agreement_enabled: true,
          privacy_policy_enabled: true,
        }}
        checked={false}
        onCheckedChange={vi.fn()}
      />
    )

    expect(
      screen.getByRole('link', { name: 'User Agreement' })
    ).toHaveAttribute('href', '/user-agreement')
    expect(
      screen.getByRole('link', { name: 'Privacy Policy' })
    ).toHaveAttribute('href', '/privacy-policy')
  })
})
