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
import type { ClientDownloadAsset, DownloadPlatform } from '../types'

/** The visitor's operating system; iOS has no installer but gets a notice. */
export type VisitorPlatform = DownloadPlatform | 'ios'

export const PLATFORM_ORDER: DownloadPlatform[] = [
  'windows',
  'macos',
  'linux',
  'android',
]

export const PLATFORM_LABELS: Record<DownloadPlatform, string> = {
  windows: 'Windows',
  macos: 'macOS',
  linux: 'Linux',
  android: 'Android',
}

/**
 * Detect the visitor's operating system from the user agent and, when the
 * browser provides it, `navigator.userAgentData.platform`.
 *
 * Android and iOS are checked first because their user agents also mention
 * Linux and Mac OS X.
 */
export function detectVisitorPlatform(
  userAgent: string,
  uaDataPlatform?: string
): VisitorPlatform | null {
  const source = `${uaDataPlatform ?? ''} ${userAgent}`.toLowerCase()
  if (/iphone|ipad|ipod/.test(source)) return 'ios'
  if (source.includes('android')) return 'android'
  if (source.includes('windows')) return 'windows'
  if (/macintosh|mac os|macos/.test(source)) return 'macos'
  if (source.includes('cros')) return null
  if (source.includes('linux')) return 'linux'
  return null
}

/** i18n key describing an installer within its platform. */
export function assetLabelKey(asset: ClientDownloadAsset): string {
  if (asset.format === 'exe') return 'Installer'
  if (asset.format === 'dmg') {
    return asset.arch === 'arm64' ? 'Apple silicon' : 'Intel chip'
  }
  if (asset.format === 'appimage') return 'AppImage'
  if (asset.format === 'deb') return 'Debian / Ubuntu (.deb)'
  if (asset.format === 'apk') return 'APK'
  return asset.format.toUpperCase()
}

/** Accelerated mirror when the site configures one, otherwise GitHub. */
export function downloadHref(asset: ClientDownloadAsset): string {
  return asset.mirror_url || asset.url
}
