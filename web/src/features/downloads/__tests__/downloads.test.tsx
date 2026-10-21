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
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { parseHeaderNavModules } from '@/lib/nav-modules'

import { DownloadsHero } from '../components/downloads-hero'
import { PlatformDownloads } from '../components/platform-downloads'
import { detectVisitorPlatform } from '../lib/platform'
import type { ClientDownloadAsset, ClientDownloadRelease } from '../types'

const GITHUB = 'https://github.com/yiranxiaohui/Pier/releases/download/v0.2.4'
const MIRROR = 'https://gh-proxy.com/'

const windowsAsset: ClientDownloadAsset = {
  name: 'pier-desktop-v0.2.4-windows-x64.setup.exe',
  platform: 'windows',
  arch: 'x64',
  format: 'exe',
  size: 10 * 1024 * 1024,
  url: `${GITHUB}/pier-desktop-v0.2.4-windows-x64.setup.exe`,
  mirror_url: `${MIRROR}${GITHUB}/pier-desktop-v0.2.4-windows-x64.setup.exe`,
  sha256: 'a'.repeat(64),
}

const macAsset: ClientDownloadAsset = {
  name: 'pier-desktop-v0.2.4-darwin-arm64.dmg',
  platform: 'macos',
  arch: 'arm64',
  format: 'dmg',
  size: 20 * 1024 * 1024,
  url: `${GITHUB}/pier-desktop-v0.2.4-darwin-arm64.dmg`,
}

const release: ClientDownloadRelease = {
  version: 'v0.2.4',
  published_at: '2026-09-28T10:00:00Z',
  notes: '',
  release_url: 'https://github.com/yiranxiaohui/Pier/releases/tag/v0.2.4',
  assets: [windowsAsset, macAsset],
}

describe('detectVisitorPlatform', () => {
  it.each([
    [
      'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36',
      undefined,
      'windows',
    ],
    [
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15',
      undefined,
      'macos',
    ],
    ['Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36', undefined, 'linux'],
    [
      'Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36',
      undefined,
      'android',
    ],
    [
      'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15',
      undefined,
      'ios',
    ],
    [
      'Mozilla/5.0 (X11; CrOS x86_64 14541.0.0) AppleWebKit/537.36',
      undefined,
      null,
    ],
    ['Mozilla/5.0 AppleWebKit/537.36', 'macOS', 'macos'],
    ['Mozilla/5.0 AppleWebKit/537.36', undefined, null],
  ])('maps %s (%s) to %s', (userAgent, uaDataPlatform, expected) => {
    expect(detectVisitorPlatform(userAgent, uaDataPlatform)).toBe(expected)
  })
})

describe('downloads navigation module', () => {
  it('is public by default and follows the saved access flags', () => {
    expect(parseHeaderNavModules('').downloads).toEqual({
      enabled: true,
      requireAuth: false,
    })
    expect(
      parseHeaderNavModules(
        JSON.stringify({ downloads: { enabled: false, requireAuth: true } })
      ).downloads
    ).toEqual({ enabled: false, requireAuth: true })
  })
})

describe('DownloadsHero', () => {
  it('opens the platform picker instead of guessing a processor for a matching device', () => {
    render(<DownloadsHero release={release} visitorPlatform='macos' />)

    expect(
      screen.getByRole('button', { name: /Download for macOS/ })
    ).toHaveAttribute('href', '#download-installers')
    expect(screen.getByText(/Version v0\.2\.4/)).toBeInTheDocument()
    expect(
      screen.getByRole('heading', {
        name: 'A cross-device workbench for coding agents.',
      })
    ).toBeInTheDocument()
  })

  it('offers all downloads when the visitor platform has no installer', () => {
    render(<DownloadsHero release={release} visitorPlatform='ios' />)

    expect(
      screen.getByRole('button', { name: 'Download Pier' })
    ).toHaveAttribute('href', '#download-installers')
  })

  it.each([undefined, { ...release, assets: [] }])(
    'keeps the project link available without suggesting an unavailable download',
    (data) => {
      render(<DownloadsHero release={data} visitorPlatform='windows' />)

      expect(
        screen.queryByRole('button', { name: /Download/ })
      ).not.toBeInTheDocument()
      expect(
        screen.getByRole('button', { name: 'Explore the project' })
      ).toHaveAttribute('href', 'https://github.com/yiranxiaohui/Pier')
    }
  )
})

describe('PlatformDownloads', () => {
  it('selects the visitor platform and changes the visible installers when another tab is clicked', async () => {
    const user = userEvent.setup()
    render(
      <PlatformDownloads assets={release.assets} visitorPlatform='macos' />
    )

    expect(screen.getByRole('tab', { name: 'macOS' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
    expect(
      within(screen.getByRole('tabpanel')).getByText('Your device')
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', {
        name: 'Download macOS · Apple silicon · arm64',
      })
    ).toHaveAttribute('href', macAsset.url)
    expect(
      screen.queryByRole('button', { name: /from GitHub/ })
    ).not.toBeInTheDocument()

    await user.click(screen.getByRole('tab', { name: 'Windows' }))
    expect(
      screen.getByRole('button', { name: 'Download Windows · Installer · x64' })
    ).toHaveAttribute('href', windowsAsset.mirror_url)
    expect(
      screen.getByRole('button', {
        name: 'Download Windows · Installer · x64 from GitHub',
      })
    ).toHaveAttribute('href', windowsAsset.url)
    expect(
      screen.queryByRole('button', {
        name: 'Download macOS · Apple silicon · arm64',
      })
    ).not.toBeInTheDocument()
    expect(screen.queryByText('Your device')).not.toBeInTheDocument()
  })

  it('supports keyboard navigation between platform tabs', async () => {
    const user = userEvent.setup()
    render(
      <PlatformDownloads assets={release.assets} visitorPlatform='windows' />
    )

    await user.tab()
    expect(screen.getByRole('tab', { name: 'Windows' })).toHaveFocus()
    await user.keyboard('{ArrowRight}')
    expect(screen.getByRole('tab', { name: 'macOS' })).toHaveFocus()
    await user.keyboard('{Enter}')
    await waitFor(() =>
      expect(screen.getByRole('tab', { name: 'macOS' })).toHaveAttribute(
        'aria-selected',
        'true'
      )
    )
    expect(screen.getByRole('tab', { name: 'macOS' })).toHaveFocus()
    expect(
      screen.getByRole('button', {
        name: 'Download macOS · Apple silicon · arm64',
      })
    ).toBeVisible()
  })

  it.each(['ios', null] as const)(
    'falls back to an available platform for %s without showing unavailable tabs',
    (visitorPlatform) => {
      render(
        <PlatformDownloads
          assets={release.assets}
          visitorPlatform={visitorPlatform}
        />
      )

      expect(screen.getByRole('tab', { name: 'Windows' })).toHaveAttribute(
        'aria-selected',
        'true'
      )
      expect(screen.getAllByRole('tab')).toHaveLength(2)
      expect(screen.queryByText('Your device')).not.toBeInTheDocument()
    }
  )

  it('falls back when a refreshed release removes the selected platform', async () => {
    const user = userEvent.setup()
    const view = render(
      <PlatformDownloads assets={release.assets} visitorPlatform='windows' />
    )
    await user.click(screen.getByRole('tab', { name: 'macOS' }))

    view.rerender(
      <PlatformDownloads assets={[windowsAsset]} visitorPlatform='windows' />
    )

    expect(screen.getByRole('tab', { name: 'Windows' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
    expect(
      screen.getByRole('button', { name: 'Download Windows · Installer · x64' })
    ).toBeVisible()
  })

  it('explains an empty release without displaying an empty tab list', () => {
    render(<PlatformDownloads assets={[]} visitorPlatform='windows' />)

    expect(screen.getByText('No installers available')).toBeInTheDocument()
    expect(screen.queryByRole('tablist')).not.toBeInTheDocument()
  })

  it('distinguishes Linux architectures in download names and copies the complete checksum', async () => {
    const user = userEvent.setup()
    const writeText = vi.spyOn(navigator.clipboard, 'writeText')
    const assets = ['x64', 'arm64'].map((arch) => ({
      ...windowsAsset,
      name: `pier-linux-${arch}.deb`,
      platform: 'linux' as const,
      format: 'deb',
      arch,
    }))
    render(<PlatformDownloads assets={assets} visitorPlatform='linux' />)

    for (const arch of ['x64', 'arm64']) {
      expect(
        screen.getByRole('button', {
          name: new RegExp(`^Download Linux · Debian .* · ${arch}$`),
        })
      ).toBeVisible()
    }
    await user.click(
      screen.getAllByRole('button', { name: 'Copy SHA-256 checksum' })[0]
    )
    expect(writeText).toHaveBeenCalledWith(windowsAsset.sha256)
    expect(screen.getByRole('button', { name: 'Copied' })).toBeInTheDocument()
  })
})
