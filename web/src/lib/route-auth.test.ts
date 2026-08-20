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
import { afterEach, describe, expect, test, vi } from 'vitest'

import { bootstrapAuthentication } from '@/lib/auth-session'
import { useAuthStore, type AuthBundle } from '@/stores/auth-store'

import { getBootstrappedAuth } from './route-auth'

vi.mock('@/lib/auth-session', () => ({
  bootstrapAuthentication: vi.fn(),
}))

const bootstrapAuthenticationMock = vi.mocked(bootstrapAuthentication)
const bundle: AuthBundle = {
  access_token: 'token',
  token_type: 'Bearer',
  access_expires_at: Math.floor(Date.now() / 1000) + 600,
  user: { id: 1, username: 'user', role: 1 },
  session: {
    sid: 'session',
    current: true,
    login_method: 'password',
    ip: '127.0.0.1',
    user_agent: 'test',
    created_at: 1,
    last_active_at: 1,
    expires_at: 1000,
  },
}

afterEach(() => {
  bootstrapAuthenticationMock.mockReset()
  useAuthStore.getState().auth.reset('idle')
})

describe('route authentication bootstrap', () => {
  test('waits for bootstrap before exposing authenticated route state', async () => {
    let resolveBootstrap: (() => void) | undefined
    bootstrapAuthenticationMock.mockReturnValue(
      new Promise((resolve) => {
        resolveBootstrap = () => resolve({ kind: 'authenticated', bundle })
      })
    )

    const result = getBootstrappedAuth()
    let settled = false
    void result.then(() => {
      settled = true
    })
    await Promise.resolve()
    expect(settled).toBe(false)

    useAuthStore.getState().auth.setBundle(bundle)
    resolveBootstrap?.()

    await expect(result).resolves.toMatchObject({
      user: bundle.user,
      accessToken: bundle.access_token,
    })
  })
})
