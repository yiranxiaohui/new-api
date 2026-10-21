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
  ArrowDown01Icon,
  ArrowUpRight01Icon,
  ComputerTerminal01Icon,
  Download04Icon,
  GitBranchIcon,
  SmartPhone01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { formatDateStr } from '@/lib/format'

import { PLATFORM_LABELS, type VisitorPlatform } from '../lib/platform'
import type { ClientDownloadRelease } from '../types'
import { PierPreview } from './pier-preview'

type DownloadsHeroProps = {
  release?: ClientDownloadRelease
  visitorPlatform: VisitorPlatform | null
}

export function DownloadsHero(props: DownloadsHeroProps) {
  const { t } = useTranslation()
  const availablePlatform = props.release?.assets.find(
    (asset) => asset.platform === props.visitorPlatform
  )?.platform
  const publishedAt = props.release?.published_at
    ? formatDateStr(new Date(props.release.published_at))
    : ''
  const features = [
    {
      icon: ComputerTerminal01Icon,
      title: t('Your agents, one workbench'),
      description: t(
        'Work with built-in pi, Claude Code and Codex in one place.'
      ),
    },
    {
      icon: SmartPhone01Icon,
      title: t('Stay connected across devices'),
      description: t(
        'Follow progress, send instructions and approve actions from your desktop or phone.'
      ),
    },
    {
      icon: GitBranchIcon,
      title: t('Your whole workspace at hand'),
      description: t(
        'Manage workspaces and sessions across computers, with files, Git and terminals built in.'
      ),
    },
  ]

  return (
    <section
      aria-labelledby='pier-heading'
      className='flex flex-col gap-8 lg:gap-12'
    >
      <div className='grid items-center gap-8 lg:grid-cols-[0.9fr_1.1fr] lg:gap-12'>
        <div className='flex min-w-0 flex-col items-start gap-6'>
          <div className='flex items-center gap-3'>
            <img
              src='/images/pier/logo.svg'
              alt=''
              width={52}
              height={52}
              className='size-13 rounded-2xl'
            />
            <div className='flex flex-col gap-1'>
              <p className='text-muted-foreground text-xs font-medium tracking-wider'>
                {t('Client Downloads')}
              </p>
              <h1
                id='pier-heading'
                className='text-3xl leading-none font-semibold tracking-tight'
              >
                Pier
              </h1>
            </div>
          </div>
          <div className='flex flex-col gap-4'>
            <h2 className='max-w-xl text-3xl leading-tight font-semibold tracking-tight text-balance sm:text-4xl lg:text-[2.75rem]'>
              {t('A cross-device workbench for coding agents.')}
            </h2>
            <p className='text-muted-foreground max-w-xl text-sm leading-7 text-pretty sm:text-base'>
              {t(
                'Your agents run on your computers. Pick up their work from any desktop or phone.'
              )}
            </p>
          </div>
          <div className='flex w-full flex-wrap gap-3'>
            {props.release && props.release.assets.length > 0 && (
              <Button
                size='lg'
                className='h-11 px-5'
                nativeButton={false}
                render={<a href='#download-installers' />}
              >
                <HugeiconsIcon
                  icon={Download04Icon}
                  aria-hidden='true'
                  data-icon='inline-start'
                />
                {availablePlatform
                  ? t('Download for {{platform}}', {
                      platform: PLATFORM_LABELS[availablePlatform],
                    })
                  : t('Download Pier')}
                <HugeiconsIcon
                  icon={ArrowDown01Icon}
                  aria-hidden='true'
                  data-icon='inline-end'
                />
              </Button>
            )}
            <Button
              variant='outline'
              size='lg'
              className='h-11 px-5'
              nativeButton={false}
              render={
                <a
                  href='https://github.com/yiranxiaohui/Pier'
                  target='_blank'
                  rel='noopener noreferrer'
                />
              }
            >
              {t('Explore the project')}
              <HugeiconsIcon
                icon={ArrowUpRight01Icon}
                aria-hidden='true'
                data-icon='inline-end'
              />
            </Button>
          </div>
          {props.release && (
            <div className='text-muted-foreground flex flex-wrap items-center gap-2 text-xs'>
              <Badge variant='secondary'>
                {t('Version {{version}}', { version: props.release.version })}
              </Badge>
              {publishedAt && (
                <time dateTime={props.release.published_at}>{publishedAt}</time>
              )}
            </div>
          )}
        </div>

        <PierPreview />
      </div>

      <div className='grid gap-6 border-y py-6 sm:grid-cols-3 sm:gap-8'>
        {features.map((feature) => (
          <div key={feature.title} className='flex gap-3'>
            <div className='bg-primary/8 text-primary flex size-9 shrink-0 items-center justify-center rounded-xl'>
              <HugeiconsIcon
                icon={feature.icon}
                className='size-5'
                aria-hidden='true'
              />
            </div>
            <div className='flex min-w-0 flex-col gap-1.5'>
              <h3 className='text-sm font-semibold'>{feature.title}</h3>
              <p className='text-muted-foreground text-xs leading-6'>
                {feature.description}
              </p>
            </div>
          </div>
        ))}
      </div>
    </section>
  )
}
