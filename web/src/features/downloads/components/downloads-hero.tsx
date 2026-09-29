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
import { Download } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { formatDateStr } from '@/lib/format'

import {
  PLATFORM_LABELS,
  downloadHref,
  type VisitorPlatform,
} from '../lib/platform'
import type { ClientDownloadRelease } from '../types'

type DownloadsHeroProps = {
  release?: ClientDownloadRelease
  visitorPlatform: VisitorPlatform | null
}

export function DownloadsHero(props: DownloadsHeroProps) {
  const { t } = useTranslation()
  const recommended = props.release?.assets.find(
    (asset) => asset.platform === props.visitorPlatform
  )
  const publishedAt = props.release?.published_at
    ? formatDateStr(new Date(props.release.published_at))
    : ''

  return (
    <section className='space-y-5'>
      <div className='space-y-2'>
        <p className='text-muted-foreground text-sm font-medium'>
          {t('Client Downloads')}
        </p>
        <h1 className='text-[clamp(1.75rem,4vw,2.5rem)] leading-[1.15] font-bold tracking-tight'>
          Pier
        </h1>
        <p className='text-muted-foreground/80 max-w-2xl text-sm'>
          {t(
            'Pier is a desktop dock and mobile remote for coding agents. Sign in with this site in one click to use its models.'
          )}
        </p>
      </div>

      {props.release && (
        <div className='flex flex-wrap items-center gap-x-4 gap-y-2'>
          {recommended && (
            <Button
              size='lg'
              nativeButton={false}
              render={<a href={downloadHref(recommended)} rel='noopener' />}
            >
              <Download aria-hidden='true' />
              {t('Download for {{platform}}', {
                platform: PLATFORM_LABELS[recommended.platform],
              })}
            </Button>
          )}
          <span className='text-muted-foreground text-sm'>
            {t('Version {{version}}', { version: props.release.version })}
            {publishedAt && ` · ${publishedAt}`}
          </span>
        </div>
      )}
    </section>
  )
}
