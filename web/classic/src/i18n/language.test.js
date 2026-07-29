/*
Copyright (C) 2025 QuantumNous

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
import assert from 'node:assert/strict';
import { describe, test } from 'node:test';
import {
  buildLanguageUrl,
  getLanguageFromSearch,
  normalizeLanguage,
} from './language';

describe('classic interface language detection', () => {
  test('normalizes browser and system locale values', () => {
    assert.equal(normalizeLanguage('en-US'), 'en');
    assert.equal(normalizeLanguage('fr-FR'), 'fr');
    assert.equal(normalizeLanguage('zhCN'), 'zh-CN');
    assert.equal(normalizeLanguage('zh-Hant-HK'), 'zh-TW');
  });

  test('reads supported languages from shared URLs', () => {
    assert.equal(getLanguageFromSearch('?lang=en'), 'en');
    assert.equal(getLanguageFromSearch('?language=ja-JP'), 'ja');
    assert.equal(getLanguageFromSearch('?lang=invalid&language=zhTW'), 'zh-TW');
    assert.equal(getLanguageFromSearch('?lang=de-DE'), undefined);
  });

  test('adds the canonical language parameter without losing URL state', () => {
    assert.equal(
      buildLanguageUrl(
        'https://example.com/pricing?vendor=openai&language=fr#models',
        'zh-TW',
      ),
      'https://example.com/pricing?vendor=openai&lang=zh-TW#models',
    );
  });
});
