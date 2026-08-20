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
import { QueryClient } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { getStatus } from '@/lib/api'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { applyCachedStatusBranding, statusQueryOptions } from '../status-query'

vi.mock('@/lib/api', () => ({
  getStatus: vi.fn(),
}))

const getStatusMock = vi.mocked(getStatus)

function createQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  })
}

describe('status query', () => {
  beforeEach(() => {
    window.localStorage.clear()
    document.head.innerHTML = '<meta name="title" content="old">'
    document.title = 'old'
    useSystemConfigStore.setState({
      config: {
        systemName: 'New API',
        logo: '/logo.png',
        currency: { ...DEFAULT_CURRENCY_CONFIG },
      },
      loading: true,
      loadedLogoUrl: '/logo.png',
    })
  })

  afterEach(() => {
    getStatusMock.mockReset()
  })

  test('deduplicates concurrent consumers and synchronizes status state', async () => {
    let resolveStatus: ((status: Record<string, unknown>) => void) | undefined
    getStatusMock.mockReturnValue(
      new Promise((resolve) => {
        resolveStatus = resolve
      })
    )
    const client = createQueryClient()

    const first = client.fetchQuery(statusQueryOptions)
    const second = client.ensureQueryData(statusQueryOptions)

    expect(getStatusMock).toHaveBeenCalledTimes(1)
    resolveStatus?.({
      system_name: 'AI Hub',
      logo: '/brand.svg',
      display_in_currency: false,
      quota_per_unit: 1000,
    })
    await expect(Promise.all([first, second])).resolves.toHaveLength(2)

    expect(getStatusMock).toHaveBeenCalledTimes(1)
    expect(client.getQueryData(statusQueryOptions.queryKey)).toMatchObject({
      system_name: 'AI Hub',
      logo: '/brand.svg',
    })
    expect(
      JSON.parse(window.localStorage.getItem('status') ?? '{}')
    ).toMatchObject({
      system_name: 'AI Hub',
      logo: '/brand.svg',
    })
    expect(useSystemConfigStore.getState()).toMatchObject({
      loading: false,
      config: {
        systemName: 'AI Hub',
        logo: '/brand.svg',
        currency: {
          displayInCurrency: false,
          quotaPerUnit: 1000,
        },
      },
    })
    expect(document.title).toBe('AI Hub')
    expect(document.querySelector('meta[name="title"]')).toHaveAttribute(
      'content',
      'AI Hub'
    )
    expect(
      document.querySelector<HTMLLinkElement>('link[rel~="icon"]')?.href
    ).toBe(new URL('/brand.svg', window.location.href).href)
  })

  test('ends system config loading when status request fails', async () => {
    window.localStorage.setItem(
      'status',
      JSON.stringify({ system_name: 'Cached Hub', logo: '/cached.svg' })
    )
    getStatusMock.mockRejectedValue(new Error('status unavailable'))
    const client = createQueryClient()

    await expect(client.fetchQuery(statusQueryOptions)).rejects.toThrow(
      'status unavailable'
    )

    expect(useSystemConfigStore.getState().loading).toBe(false)
    expect(JSON.parse(window.localStorage.getItem('status') ?? '{}')).toEqual({
      system_name: 'Cached Hub',
      logo: '/cached.svg',
    })
    expect(useSystemConfigStore.getState().config.systemName).toBe('New API')
  })

  test('applies cached branding without requesting status', () => {
    window.localStorage.setItem(
      'status',
      JSON.stringify({ system_name: 'Cached Hub', logo: '/cached.svg' })
    )

    applyCachedStatusBranding()

    expect(getStatusMock).not.toHaveBeenCalled()
    expect(document.title).toBe('Cached Hub')
    expect(
      document.querySelector<HTMLLinkElement>('link[rel~="icon"]')?.href
    ).toBe(new URL('/cached.svg', window.location.href).href)
  })
})
