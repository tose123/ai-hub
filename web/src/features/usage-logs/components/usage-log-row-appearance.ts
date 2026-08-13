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
import { LOG_TYPE_ENUM } from '../constants'
import type { UsageLog } from '../data/schema'
import { parseLogOther } from '../lib/format'

type CommonLogRowSurface = 'table' | 'mobile'
type CommonLogRowData = Pick<UsageLog, 'type' | 'should_retry' | 'other'>

const logTypeRowClassNames: Record<
  CommonLogRowSurface,
  Record<number, string>
> = {
  table: {
    [LOG_TYPE_ENUM.ERROR]: 'bg-rose-50/40 dark:bg-rose-950/20',
    [LOG_TYPE_ENUM.REFUND]: 'bg-blue-50/30 dark:bg-blue-950/15',
  },
  mobile: {
    [LOG_TYPE_ENUM.ERROR]:
      'border-rose-200/50 bg-rose-50/40 dark:border-rose-900/30 dark:bg-rose-950/20',
    [LOG_TYPE_ENUM.REFUND]:
      'border-blue-200/50 bg-blue-50/30 dark:border-blue-900/30 dark:bg-blue-950/15',
  },
}

const retryRowClassNames: Record<CommonLogRowSurface, string> = {
  table:
    'text-muted-foreground [&>td]:opacity-70 [&>td]:transition-opacity hover:[&>td]:opacity-90 focus-within:[&>td]:opacity-90',
  mobile:
    'border-border/40 text-muted-foreground opacity-70 hover:opacity-90 focus-within:opacity-90',
}

const quotaSaturationRowClassNames: Record<CommonLogRowSurface, string> = {
  table: 'bg-warning/10',
  mobile: 'border-warning/30 bg-warning/10',
}

export function getCommonLogRowClassName(
  log: CommonLogRowData,
  surface: CommonLogRowSurface,
  isAdmin: boolean
): string {
  if (isAdmin) {
    const other = parseLogOther(log.other)
    if (other?.admin_info?.quota_saturation) {
      return quotaSaturationRowClassNames[surface]
    }
    if (log.should_retry === 1) {
      return retryRowClassNames[surface]
    }
  }

  return logTypeRowClassNames[surface][log.type] ?? ''
}
