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
import { ChevronDown, ExternalLink } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { RichContent } from '@/components/rich-content'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'

import type { ClientDownloadRelease } from '../types'

export function ReleaseNotes(props: { release: ClientDownloadRelease }) {
  const { t } = useTranslation()
  const notes = props.release.notes.trim()

  return (
    <Collapsible className='rounded-xl border'>
      <div className='flex flex-wrap items-center justify-between gap-2 px-4 py-2'>
        <CollapsibleTrigger
          disabled={notes === ''}
          render={
            <Button
              variant='ghost'
              className='group/notes px-0 hover:bg-transparent aria-expanded:bg-transparent dark:hover:bg-transparent'
            />
          }
        >
          {t('Release notes for {{version}}', {
            version: props.release.version,
          })}
          <ChevronDown
            aria-hidden='true'
            className='transition-transform group-aria-expanded/notes:rotate-180'
          />
        </CollapsibleTrigger>
        {props.release.release_url && (
          <Button
            variant='link'
            size='sm'
            nativeButton={false}
            render={
              <a
                href={props.release.release_url}
                target='_blank'
                rel='noopener noreferrer'
              />
            }
          >
            {t('View on GitHub')}
            <ExternalLink aria-hidden='true' />
          </Button>
        )}
      </div>
      <CollapsibleContent className='border-t px-4 py-3'>
        <RichContent content={notes} className='text-sm' />
      </CollapsibleContent>
    </Collapsible>
  )
}
