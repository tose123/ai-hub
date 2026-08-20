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
import { queryOptions } from '@tanstack/react-query'

import type { SystemStatus } from '@/features/auth/types'
import { getStatus } from '@/lib/api'
import { DEFAULT_SYSTEM_NAME, DEFAULT_LOGO } from '@/lib/constants'
import { applyFaviconToDom } from '@/lib/dom-utils'
import {
  useSystemConfigStore,
  type CurrencyConfig,
  type CurrencyDisplayType,
  type SystemConfig,
  DEFAULT_CURRENCY_CONFIG,
} from '@/stores/system-config-store'

interface StatusConfigData {
  system_name?: string
  logo?: string
  footer_html?: string
  demo_site_enabled?: boolean
  display_token_stat_enabled?: boolean
  display_in_currency?: boolean
  quota_display_type?: CurrencyDisplayType | string
  quota_per_unit?: number
  usd_exchange_rate?: number
  custom_currency_symbol?: string
  custom_currency_exchange_rate?: number
}

function toNumber(value: unknown, fallback: number): number {
  if (typeof value === 'number' && !Number.isNaN(value)) return value
  if (typeof value === 'string') {
    const parsed = Number(value)
    if (!Number.isNaN(parsed)) return parsed
  }
  return fallback
}

export function mapStatusDataToConfig(
  data: StatusConfigData | undefined
): Partial<SystemConfig> {
  if (!data) return {}

  const quotaDisplayType =
    (data.quota_display_type as CurrencyDisplayType | undefined) ??
    DEFAULT_CURRENCY_CONFIG.quotaDisplayType

  const currency: CurrencyConfig = {
    displayInCurrency:
      data.display_in_currency ?? DEFAULT_CURRENCY_CONFIG.displayInCurrency,
    quotaDisplayType,
    quotaPerUnit: toNumber(
      data.quota_per_unit,
      DEFAULT_CURRENCY_CONFIG.quotaPerUnit
    ),
    usdExchangeRate: toNumber(
      data.usd_exchange_rate,
      DEFAULT_CURRENCY_CONFIG.usdExchangeRate
    ),
    customCurrencySymbol:
      data.custom_currency_symbol?.trim() ||
      DEFAULT_CURRENCY_CONFIG.customCurrencySymbol,
    customCurrencyExchangeRate: toNumber(
      data.custom_currency_exchange_rate,
      DEFAULT_CURRENCY_CONFIG.customCurrencyExchangeRate
    ),
  }

  return {
    systemName: data.system_name || DEFAULT_SYSTEM_NAME,
    logo: data.logo || DEFAULT_LOGO,
    footerHtml: data.footer_html,
    demoSiteEnabled: data.demo_site_enabled,
    displayTokenStatEnabled: data.display_token_stat_enabled,
    currency,
  }
}

export function getCachedStatus(): SystemStatus | undefined {
  try {
    if (typeof window === 'undefined') return undefined
    const saved = window.localStorage.getItem('status')
    return saved ? (JSON.parse(saved) as SystemStatus) : undefined
  } catch {
    return undefined
  }
}

function cacheStatus(status: SystemStatus): void {
  try {
    if (typeof window !== 'undefined') {
      window.localStorage.setItem('status', JSON.stringify(status))
    }
  } catch {
    /* empty */
  }
}

function applySystemNameToDom(systemName: string): void {
  if (typeof document === 'undefined') return
  document.title = systemName
  const metaTitle =
    document.querySelector<HTMLMetaElement>('meta[name="title"]')
  metaTitle?.setAttribute('content', systemName)
}

export function applyStatusBranding(status: SystemStatus | undefined): void {
  if (!status) return
  if (typeof status.system_name === 'string' && status.system_name) {
    applySystemNameToDom(status.system_name)
  }
  if (typeof status.logo === 'string' && status.logo) {
    applyFaviconToDom(status.logo)
  }
}

export function applyCachedStatusBranding(): void {
  applyStatusBranding(getCachedStatus())
}

function synchronizeStatus(status: SystemStatus): void {
  useSystemConfigStore.getState().setConfig(mapStatusDataToConfig(status))
  cacheStatus(status)
  applyStatusBranding(status)
}

async function fetchAndSynchronizeStatus(): Promise<SystemStatus> {
  try {
    const status = (await getStatus()) as SystemStatus
    synchronizeStatus(status)
    return status
  } finally {
    useSystemConfigStore.getState().setLoading(false)
  }
}

export const statusQueryOptions = queryOptions({
  queryKey: ['status'] as const,
  queryFn: fetchAndSynchronizeStatus,
  placeholderData: getCachedStatus,
  staleTime: 5 * 60 * 1000,
  gcTime: 30 * 60 * 1000,
})
