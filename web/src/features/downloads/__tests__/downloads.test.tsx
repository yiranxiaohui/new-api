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
import { render, screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

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
  it('recommends the installer for the visitor platform through the mirror', () => {
    render(<DownloadsHero release={release} visitorPlatform='windows' />)

    const link = screen.getByRole('button', { name: /Download for Windows/ })
    expect(link).toHaveAttribute('href', windowsAsset.mirror_url)
    expect(screen.getByText(/Version v0\.2\.4/)).toBeInTheDocument()
  })

  it('omits the recommendation when no installer matches the visitor', () => {
    render(<DownloadsHero release={release} visitorPlatform='ios' />)

    expect(
      screen.queryByRole('button', { name: /Download for/ })
    ).not.toBeInTheDocument()
  })
})

describe('PlatformDownloads', () => {
  it('offers the mirror and GitHub links only for mirrored installers', () => {
    render(
      <PlatformDownloads assets={release.assets} visitorPlatform='macos' />
    )

    expect(
      screen.getByRole('button', { name: 'Download Installer' })
    ).toHaveAttribute('href', windowsAsset.mirror_url)
    expect(
      screen.getByRole('button', { name: 'Download Installer from GitHub' })
    ).toHaveAttribute('href', windowsAsset.url)
    expect(
      screen.getByRole('button', { name: 'Download Apple silicon' })
    ).toHaveAttribute('href', macAsset.url)
    expect(
      screen.queryByRole('button', {
        name: 'Download Apple silicon from GitHub',
      })
    ).not.toBeInTheDocument()
  })

  it('marks only the visitor platform and skips platforms without installers', () => {
    render(
      <PlatformDownloads assets={release.assets} visitorPlatform='macos' />
    )

    const headings = screen.getAllByRole('heading', { level: 2 })
    expect(headings.map((heading) => heading.textContent)).toEqual([
      'Windows',
      'macOS',
    ])
    const macCard = headings[1].closest('[data-slot="card"]') as HTMLElement
    expect(within(macCard).getByText('Your device')).toBeInTheDocument()
    expect(screen.getAllByText('Your device')).toHaveLength(1)
  })
})
