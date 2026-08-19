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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, test, vi } from 'vitest'

import type { Channel } from '../../../types'

const currentRow: Channel = {
  id: 1,
  type: 1,
  key: 'sk-test',
  status: 1,
  name: 'test-channel',
  created_time: 0,
  test_time: 0,
  response_time: 0,
  other: '',
  balance: 0,
  balance_updated_time: 0,
  models: '',
  group: 'default',
  used_quota: 0,
  other_info: '',
  remark: '',
  max_input_tokens: 0,
  channel_info: {
    is_multi_key: false,
    multi_key_size: 0,
    multi_key_polling_index: 0,
    multi_key_mode: 'random',
  },
  settings: '{}',
}

vi.mock('../../channels-provider', () => ({
  useChannels: () => ({ currentRow }),
}))

const { ChannelTestDialog } = await import('../channel-test-dialog')

function TestHarness() {
  const [open, setOpen] = useState(true)
  const [queryClient] = useState(() => new QueryClient())

  return (
    <QueryClientProvider client={queryClient}>
      <button type='button' onClick={() => setOpen(true)}>
        Open channel test
      </button>
      <ChannelTestDialog
        open={open}
        onOpenChange={(nextOpen) => setOpen(nextOpen)}
      />
    </QueryClientProvider>
  )
}

describe('ChannelTestDialog stream default', () => {
  test('restores streaming after closing and reopening the same channel', async () => {
    const user = userEvent.setup()
    render(<TestHarness />)

    expect(screen.getByRole('switch', { name: 'Stream Mode' })).toBeChecked()

    const closeButtons = screen.getAllByRole('button', { name: 'Close' })
    await user.click(closeButtons.at(-1)!)
    await user.click(screen.getByRole('button', { name: 'Open channel test' }))

    expect(screen.getByRole('switch', { name: 'Stream Mode' })).toBeChecked()
  })
})
