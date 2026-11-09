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
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { resolveWebhookFormat } from './webhook-format.ts'

const robotURL = 'https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=test-key'

test('explicit webhook format controls delivery regardless of the URL', () => {
  assert.equal(resolveWebhookFormat('generic', robotURL), 'generic')
  assert.equal(
    resolveWebhookFormat('wecom', 'https://proxy.example.com/robot'),
    'wecom'
  )
})

test('legacy configurations retain their existing delivery format', () => {
  assert.equal(resolveWebhookFormat(undefined, robotURL), 'wecom')
  assert.equal(
    resolveWebhookFormat('', 'https://example.com/webhook'),
    'generic'
  )
  assert.equal(resolveWebhookFormat(), 'generic')
  assert.equal(
    resolveWebhookFormat(
      '',
      robotURL.replace('qyapi.weixin.qq.com', 'example.com')
    ),
    'generic'
  )
  assert.equal(
    resolveWebhookFormat('', robotURL.replace('?key=test-key', '')),
    'generic'
  )
})
