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
import { describe, test } from 'node:test'
import {
  buildInterfaceLanguageUrl,
  convertDetectedLanguage,
  getInterfaceLanguageFromSearch,
  matchInterfaceLanguage,
} from './languages'

describe('interface language detection', () => {
  test('normalizes supported browser and system locales', () => {
    assert.equal(matchInterfaceLanguage('en-US'), 'en')
    assert.equal(matchInterfaceLanguage('fr-FR'), 'fr')
    assert.equal(matchInterfaceLanguage('zh-Hans-CN'), 'zhCN')
    assert.equal(matchInterfaceLanguage('zh-Hant-HK'), 'zhTW')
    assert.equal(matchInterfaceLanguage('zhCN'), 'zhCN')
    assert.equal(matchInterfaceLanguage('zhTW'), 'zhTW')
    assert.equal(matchInterfaceLanguage('de-DE'), undefined)
  })

  test('uses lang first and supports language as an alias', () => {
    assert.equal(getInterfaceLanguageFromSearch('?lang=en'), 'en')
    assert.equal(getInterfaceLanguageFromSearch('?language=ja-JP'), 'ja')
    assert.equal(
      getInterfaceLanguageFromSearch('?lang=invalid&language=zh-TW'),
      'zhTW'
    )
  })

  test('leaves unsupported detector values available for later fallbacks', () => {
    assert.equal(convertDetectedLanguage('vi-VN'), 'vi')
    assert.equal(convertDetectedLanguage('unsupported'), 'unsupported')
  })

  test('adds the canonical language parameter without losing URL state', () => {
    assert.equal(
      buildInterfaceLanguageUrl(
        'https://example.com/pricing?vendor=openai&language=fr#models',
        'zh-TW'
      ),
      'https://example.com/pricing?vendor=openai&lang=zh-TW#models'
    )
  })
})
