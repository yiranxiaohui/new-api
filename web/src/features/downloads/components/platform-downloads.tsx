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
  Download04Icon,
  LinkSquare01Icon,
  WindowsNewIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon, type IconSvgElement } from '@hugeicons/react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { EmptyState } from '@/components/empty-state'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

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
  const [selectedPlatform, setSelectedPlatform] = useState<string | null>(
    props.visitorPlatform
  )
  const platforms = PLATFORM_ORDER.filter((platform) =>
    props.assets.some((asset) => asset.platform === platform)
  )
  const activePlatform =
    platforms.find((platform) => platform === selectedPlatform) ?? platforms[0]

  return (
    <section
      id='download-installers'
      aria-labelledby='downloads-heading'
      className='flex scroll-mt-24 flex-col gap-5'
    >
      <div className='flex flex-col gap-2'>
        <h2
          id='downloads-heading'
          className='text-2xl font-semibold tracking-tight'
        >
          {t('Choose your platform')}
        </h2>
        <p className='text-muted-foreground text-sm leading-relaxed'>
          {t(
            'Choose the installer that matches your operating system and processor.'
          )}
        </p>
      </div>
      {platforms.length === 0 ? (
        <EmptyState
          bordered
          title={t('No installers available')}
          description={t(
            'Visit the Pier project on GitHub for the latest release information.'
          )}
        />
      ) : (
        <Tabs
          value={activePlatform}
          onValueChange={setSelectedPlatform}
          className='gap-5'
        >
          <TabsList
            aria-label={t('Download platform')}
            className={cn(
              'grid w-full grid-cols-2 gap-1 p-1 group-data-horizontal/tabs:h-auto sm:flex sm:w-fit',
              platforms.length === 1 && 'grid-cols-1'
            )}
          >
            {platforms.map((platform) => (
              <TabsTrigger
                key={platform}
                value={platform}
                className='min-h-11 gap-2 px-3 sm:min-w-32 sm:px-5'
              >
                <HugeiconsIcon
                  icon={PLATFORM_ICONS[platform]}
                  aria-hidden='true'
                />
                {PLATFORM_LABELS[platform]}
              </TabsTrigger>
            ))}
          </TabsList>
          {platforms.map((platform) => (
            <TabsContent key={platform} value={platform}>
              <Card className='gap-0 rounded-2xl py-0'>
                <CardHeader className='bg-muted/40 gap-2 border-b p-5 sm:px-6'>
                  <CardTitle className='flex flex-wrap items-center gap-3'>
                    <HugeiconsIcon
                      icon={PLATFORM_ICONS[platform]}
                      className='size-8'
                      aria-hidden='true'
                    />
                    <h3>{PLATFORM_LABELS[platform]}</h3>
                    {platform === props.visitorPlatform && (
                      <Badge variant='secondary'>{t('Your device')}</Badge>
                    )}
                  </CardTitle>
                  <CardDescription>
                    {t('Select a version below to download Pier.')}
                  </CardDescription>
                </CardHeader>
                <CardContent className='divide-y px-5 sm:px-6'>
                  {props.assets
                    .filter((asset) => asset.platform === platform)
                    .map((asset) => (
                      <AssetRow key={asset.name} asset={asset} />
                    ))}
                </CardContent>
              </Card>
            </TabsContent>
          ))}
        </Tabs>
      )}
    </section>
  )
}

function AssetRow(props: { asset: ClientDownloadAsset }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const asset = props.asset
  const label = t(assetLabelKey(asset))
  const downloadName = `${PLATFORM_LABELS[asset.platform]} · ${label} · ${asset.arch}`

  return (
    <div className='flex min-w-0 flex-col gap-3 py-5 sm:flex-row sm:items-center sm:justify-between sm:gap-6'>
      <div className='flex min-w-0 flex-col gap-2'>
        <div className='flex flex-wrap items-center gap-2 font-medium'>
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
                className='size-8'
                iconClassName='size-3'
                tooltip={t('Copy SHA-256 checksum')}
              />
            </>
          )}
        </div>
      </div>
      <div className='flex shrink-0 flex-wrap items-center gap-2'>
        {asset.mirror_url && (
          <Button
            variant='ghost'
            size='lg'
            nativeButton={false}
            render={<a href={asset.url} rel='noopener' />}
            aria-label={t('Download {{name}} from GitHub', {
              name: downloadName,
            })}
          >
            GitHub
            <HugeiconsIcon
              icon={LinkSquare01Icon}
              data-icon='inline-end'
              aria-hidden='true'
            />
          </Button>
        )}
        <Button
          size='lg'
          nativeButton={false}
          render={<a href={downloadHref(asset)} rel='noopener' />}
          aria-label={t('Download {{name}}', { name: downloadName })}
        >
          <HugeiconsIcon
            icon={Download04Icon}
            data-icon='inline-start'
            aria-hidden='true'
          />
          {t('Download')}
        </Button>
      </div>
    </div>
  )
}
