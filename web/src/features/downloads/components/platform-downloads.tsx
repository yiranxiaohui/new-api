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
import {
  AndroidIcon,
  AppleIcon,
  ComputerTerminal01Icon,
  WindowsNewIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon, type IconSvgElement } from '@hugeicons/react'
import { Download } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import {
  PLATFORM_LABELS,
  PLATFORM_ORDER,
  assetLabelKey,
  downloadHref,
  type VisitorPlatform,
} from '../lib/platform'
import type { ClientDownloadAsset, DownloadPlatform } from '../types'

const PLATFORM_ICONS: Record<DownloadPlatform, IconSvgElement> = {
  windows: WindowsNewIcon,
  macos: AppleIcon,
  linux: ComputerTerminal01Icon,
  android: AndroidIcon,
}

type PlatformDownloadsProps = {
  assets: ClientDownloadAsset[]
  visitorPlatform: VisitorPlatform | null
}

export function PlatformDownloads(props: PlatformDownloadsProps) {
  const { t } = useTranslation()

  return (
    <section className='grid gap-4 md:grid-cols-2'>
      {PLATFORM_ORDER.map((platform) => {
        const assets = props.assets.filter(
          (asset) => asset.platform === platform
        )
        if (assets.length === 0) return null
        return (
          <Card key={platform}>
            <CardHeader>
              <CardTitle className='flex items-center gap-2 text-base'>
                <HugeiconsIcon
                  icon={PLATFORM_ICONS[platform]}
                  className='size-5'
                  aria-hidden='true'
                />
                <h2>{PLATFORM_LABELS[platform]}</h2>
                {platform === props.visitorPlatform && (
                  <Badge variant='secondary'>{t('Your device')}</Badge>
                )}
              </CardTitle>
            </CardHeader>
            <CardContent className='divide-y'>
              {assets.map((asset) => (
                <AssetRow key={asset.name} asset={asset} />
              ))}
            </CardContent>
          </Card>
        )
      })}
    </section>
  )
}

function AssetRow(props: { asset: ClientDownloadAsset }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const asset = props.asset
  const label = t(assetLabelKey(asset))

  return (
    <div className='flex flex-col gap-2 py-3 first:pt-0 last:pb-0 sm:flex-row sm:items-center sm:justify-between'>
      <div className='min-w-0 space-y-1'>
        <div className='flex items-center gap-2 font-medium'>
          {label}
          {asset.arch !== 'universal' && (
            <Badge variant='outline'>{asset.arch}</Badge>
          )}
        </div>
        <div className='text-muted-foreground flex min-w-0 items-center gap-1 text-xs'>
          <span className='shrink-0'>
            {formatNumber(asset.size / 1024 / 1024, locale)} MB
          </span>
          {asset.sha256 && (
            <>
              <span aria-hidden='true'>·</span>
              <span className='truncate font-mono' title={asset.sha256}>
                SHA-256 {asset.sha256.slice(0, 12)}…
              </span>
              <CopyButton
                value={asset.sha256}
                className='size-6'
                iconClassName='size-3'
                tooltip={t('Copy SHA-256 checksum')}
              />
            </>
          )}
        </div>
      </div>
      <div className='flex shrink-0 items-center gap-2'>
        {asset.mirror_url && (
          <Button
            variant='ghost'
            size='sm'
            nativeButton={false}
            render={<a href={asset.url} rel='noopener' />}
            aria-label={t('Download {{name}} from GitHub', { name: label })}
          >
            GitHub
          </Button>
        )}
        <Button
          size='sm'
          nativeButton={false}
          render={<a href={downloadHref(asset)} rel='noopener' />}
          aria-label={t('Download {{name}}', { name: label })}
        >
          <Download aria-hidden='true' />
          {t('Download')}
        </Button>
      </div>
    </div>
  )
}
