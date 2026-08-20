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
import { getSetupStatus } from '@/features/setup/api'

const SETUP_CHECKED_KEY = 'setup_status_checked'

export type SetupCheckResult = 'initialized' | 'requires_setup' | 'unavailable'

type SetupStatusLoader = typeof getSetupStatus

interface SetupStatusCheckerOptions {
  load: SetupStatusLoader
  readChecked: () => boolean
  writeChecked: () => void
}

function readSetupStatusCache(): boolean {
  try {
    return window.localStorage.getItem(SETUP_CHECKED_KEY) === 'true'
  } catch {
    return false
  }
}

function writeSetupStatusCache(): void {
  try {
    window.localStorage.setItem(SETUP_CHECKED_KEY, 'true')
  } catch {
    /* empty */
  }
}

export function createSetupStatusChecker(
  options: SetupStatusCheckerOptions
): () => Promise<SetupCheckResult> {
  let checked = options.readChecked()
  let pending: Promise<SetupCheckResult> | null = null

  return () => {
    if (checked) return Promise.resolve('initialized')
    if (pending) return pending

    pending = options
      .load()
      .then((response): SetupCheckResult => {
        if (!response.success || !response.data) return 'unavailable'
        if (!response.data.status) return 'requires_setup'

        checked = true
        options.writeChecked()
        return 'initialized'
      })
      .catch(() => 'unavailable' as const)
      .finally(() => {
        pending = null
      })

    return pending
  }
}

const checkSetupStatus = createSetupStatusChecker({
  load: getSetupStatus,
  readChecked: readSetupStatusCache,
  writeChecked: writeSetupStatusCache,
})

export function checkSetupStatusInBackground(): Promise<SetupCheckResult> {
  return checkSetupStatus()
}
