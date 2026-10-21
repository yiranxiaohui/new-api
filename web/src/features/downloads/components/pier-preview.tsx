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
  ComputerTerminal01Icon,
  SmartPhone01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

// Screenshots and logo from yiranxiaohui/Pier at be858a22392b01359e56ce0cc59fc9e93927e32d.
// Preview images are resized WebP copies of the project's README screenshots.
export function PierPreview() {
  const { t } = useTranslation()

  return (
    <figure className='min-w-0'>
      <div className='from-primary/10 via-muted/60 to-primary/5 relative rounded-3xl border bg-gradient-to-br p-4 pb-8 sm:p-6 sm:pb-10'>
        <div className='bg-background/80 text-muted-foreground mb-5 flex w-fit items-center gap-2 rounded-full border px-3 py-1.5 text-xs'>
          <HugeiconsIcon
            icon={ComputerTerminal01Icon}
            className='size-4'
            aria-hidden='true'
          />
          <span>{t('Desktop')}</span>
          <span aria-hidden='true'>↔</span>
          <HugeiconsIcon
            icon={SmartPhone01Icon}
            className='size-4'
            aria-hidden='true'
          />
          <span>{t('Mobile')}</span>
        </div>
        <img
          src='/images/pier/desktop-workbench.webp'
          alt={t(
            'Pier desktop workbench with agent sessions, code changes and workspace files'
          )}
          width={1200}
          height={800}
          fetchPriority='high'
          className='ring-foreground/10 w-[94%] rounded-lg shadow-xl ring-1'
        />
        <img
          src='/images/pier/mobile-chat.webp'
          alt={t('Pier mobile app with agent progress and action approvals')}
          width={360}
          height={780}
          className='ring-background absolute right-3 bottom-3 w-[23%] rounded-xl shadow-xl ring-4 sm:right-4 sm:bottom-4'
        />
      </div>
      <figcaption className='text-muted-foreground mt-3 text-center text-xs leading-relaxed'>
        {t('Desktop and mobile previews from the Pier project')}
      </figcaption>
    </figure>
  )
}
