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
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'

export function ModelStatus() {
  const { t } = useTranslation()

  return (
    <PublicLayout showMainContainer={false}>
      <main className='h-svh pt-16'>
        {/* oxlint-disable-next-line react/iframe-missing-sandbox -- Same-origin status assets require the page's real origin. */}
        <iframe
          className='block h-[calc(100svh-4rem)] w-full border-0'
          src='/model-status-embed/'
          title={t('Model Status')}
        />
      </main>
    </PublicLayout>
  )
}
