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
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { GeneralError } from '../general-error'

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
  useRouter: () => ({ history: { go: vi.fn() } }),
}))

const reload = vi.fn()
const originalLocation = window.location

function chunkLoadError(): Error {
  const error = new Error(
    'Loading chunk 1234 failed.\n(missing: https://example.test/static/js/async/1234.abc.js)'
  )
  error.name = 'ChunkLoadError'
  return error
}

beforeEach(() => {
  sessionStorage.clear()
  Object.defineProperty(window, 'location', {
    configurable: true,
    value: { ...originalLocation, reload },
  })
})

afterEach(() => {
  Object.defineProperty(window, 'location', {
    configurable: true,
    value: originalLocation,
  })
  reload.mockReset()
  vi.useRealTimers()
})

it('a missing chunk from a previous build reloads the page without showing the error page', () => {
  render(<GeneralError error={chunkLoadError()} />)

  expect(reload).toHaveBeenCalledTimes(1)
  expect(screen.queryByText('500')).not.toBeInTheDocument()
})

it('a CSS chunk failure also reloads the page', () => {
  const error = Object.assign(new Error('Loading CSS chunk 99 failed.'), {
    code: 'CSS_CHUNK_LOAD_FAILED',
  })

  render(<GeneralError error={error} />)

  expect(reload).toHaveBeenCalledTimes(1)
})

it('a repeated chunk failure right after reloading shows the error page instead of looping', () => {
  vi.useFakeTimers({ now: 1_000_000 })
  sessionStorage.setItem('app:stale-build-reload-at', String(1_000_000 - 2_000))

  render(<GeneralError error={chunkLoadError()} />)

  expect(reload).not.toHaveBeenCalled()
  expect(screen.getByText('500')).toBeInTheDocument()
})

it('a chunk failure long after the previous reload reloads again', () => {
  vi.useFakeTimers({ now: 1_000_000 })
  sessionStorage.setItem(
    'app:stale-build-reload-at',
    String(1_000_000 - 60_000)
  )

  render(<GeneralError error={chunkLoadError()} />)

  expect(reload).toHaveBeenCalledTimes(1)
})

it('an ordinary render error shows the error page without reloading', () => {
  render(<GeneralError error={new TypeError('x is undefined')} />)

  expect(reload).not.toHaveBeenCalled()
  expect(screen.getByText('500')).toBeInTheDocument()
})
