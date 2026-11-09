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
import type { i18n, ResourceLanguage } from 'i18next'
import {
  normalizeInterfaceLanguage,
  type InterfaceLanguageCode,
} from './languages'

export type LanguageImports = Record<
  InterfaceLanguageCode,
  () => Promise<{ default: { translation: ResourceLanguage } }>
>

// Load before changing i18next's language: failed downloads must not change the
// visible language or the language detector's persisted preference.
export function createLanguageLoader(instance: i18n, imports: LanguageImports) {
  const pending = new Map<InterfaceLanguageCode, Promise<void>>()
  let latestChange = 0

  function load(language: InterfaceLanguageCode): Promise<void> {
    if (instance.hasResourceBundle(language, 'translation')) {
      return Promise.resolve()
    }
    const existing = pending.get(language)
    if (existing) return existing

    const request = imports[language]()
      .then((resource) => {
        instance.addResourceBundle(
          language,
          'translation',
          resource.default.translation
        )
      })
      .finally(() => {
        // In particular, don't cache failures: retry should download again.
        pending.delete(language)
      })
    pending.set(language, request)
    return request
  }

  async function change(language?: string): Promise<boolean> {
    const nextLanguage = normalizeInterfaceLanguage(language)
    const changeId = ++latestChange
    try {
      await Promise.all([load('en'), load(nextLanguage)])
    } catch (error) {
      if (changeId !== latestChange) return false
      throw error
    }
    // A slower, earlier selection must not overwrite the latest selection.
    if (changeId !== latestChange) return false
    await instance.changeLanguage(nextLanguage)
    return true
  }

  return { change }
}
