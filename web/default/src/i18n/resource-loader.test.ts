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
import { createInstance } from 'i18next'
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { bootstrapResources } from './bootstrap.ts'
import {
  INTERFACE_LANGUAGE_OPTIONS,
  type InterfaceLanguageCode,
} from './languages.ts'
import {
  createLanguageLoader,
  type LanguageImports,
} from './resource-loader.ts'

function resource(label: string) {
  return { default: { translation: { label } } }
}

function deferredResource() {
  let resolve!: (value: ReturnType<typeof resource>) => void
  let reject!: (reason: Error) => void
  const promise = new Promise<ReturnType<typeof resource>>(
    (onResolve, onReject) => {
      resolve = onResolve
      reject = onReject
    }
  )
  return { promise, resolve, reject }
}

async function setup(language = 'en') {
  const instance = createInstance()
  const cached: string[] = []
  instance.use({
    type: 'languageDetector',
    detect: () => language,
    cacheUserLanguage: (value: string) => {
      cached.push(value)
    },
  })
  await instance.init({
    resources: structuredClone(bootstrapResources),
    ns: ['translation', 'bootstrap'],
    defaultNS: 'translation',
    fallbackLng: 'en',
    supportedLngs: INTERFACE_LANGUAGE_OPTIONS.map((option) => option.code),
    load: 'currentOnly',
    initAsync: false,
  })
  cached.length = 0
  const requested: string[] = []
  const load = async (code: InterfaceLanguageCode) => {
    requested.push(code)
    return resource(code)
  }
  const imports: LanguageImports = {
    en: () => load('en'),
    zhCN: () => load('zhCN'),
    zhTW: () => load('zhTW'),
    fr: () => load('fr'),
    ru: () => load('ru'),
    ja: () => load('ja'),
    vi: () => load('vi'),
  }
  const loader = createLanguageLoader(instance, imports)
  return { instance, cached, requested, imports, loader }
}

test('first visit loads only the selected language and English fallback', async () => {
  for (const { code } of INTERFACE_LANGUAGE_OPTIONS) {
    const { instance, requested, loader } = await setup(code)
    await loader.change(code)
    assert.deepEqual(requested, code === 'en' ? ['en'] : ['en', code])
    assert.equal(instance.t('label'), code)
    assert.equal(
      instance.t('retry', { ns: 'bootstrap' }),
      bootstrapResources[code].bootstrap.retry
    )
  }
})

test('waits for both resources before switching and retains English fallback', async () => {
  const { instance, imports, loader, cached } = await setup()
  const english = deferredResource()
  const chinese = deferredResource()
  imports.en = () => english.promise
  imports.zhCN = () => chinese.promise
  const change = loader.change('zh-CN')
  chinese.resolve(resource('中文'))
  await chinese.promise
  assert.equal(instance.language, 'en')
  assert.deepEqual(cached, [])
  english.resolve({
    default: { translation: { label: 'English', fallbackOnly: 'Fallback' } },
  } as ReturnType<typeof resource>)
  await change
  assert.equal(instance.language, 'zhCN')
  assert.equal(instance.t('label'), '中文')
  assert.equal(instance.t('fallbackOnly'), 'Fallback')
  assert.deepEqual(cached, ['zhCN'])
})

test('failed startup or switching can retry without changing the saved preference', async () => {
  for (const failedLanguage of ['en', 'ja'] as const) {
    const { instance, imports, loader, cached } = await setup()
    let attempts = 0
    imports[failedLanguage] = async () => {
      attempts++
      if (attempts === 1) throw new Error('offline')
      return resource(failedLanguage)
    }
    await assert.rejects(loader.change('ja'), /offline/)
    assert.equal(instance.language, 'en')
    assert.deepEqual(cached, [])
    assert.equal(
      instance.t('error', { ns: 'bootstrap' }),
      bootstrapResources.en.bootstrap.error
    )
    await loader.change('ja')
    assert.equal(instance.language, 'ja')
    assert.equal(instance.t('label'), 'ja')
    assert.equal(attempts, 2)
    assert.deepEqual(cached, ['ja'])
  }
})

test('already loaded locales and concurrent downloads are reused', async () => {
  const { loader, requested } = await setup()
  const first = loader.change('zhCN')
  const second = loader.change('zhCN')
  assert.deepEqual(await Promise.all([first, second]), [false, true])
  await loader.change('en')
  await loader.change('zhCN')
  await loader.change('fr')
  await loader.change('fr')
  assert.deepEqual(requested, ['en', 'zhCN', 'fr'])
})

test('an older slow or failed selection cannot overwrite a newer selection', async () => {
  for (const fail of [false, true]) {
    const { instance, imports, loader, cached } = await setup()
    const japanese = deferredResource()
    imports.ja = () => japanese.promise
    const oldChange = loader.change('ja')
    assert.equal(await loader.change('fr'), true)
    if (fail) japanese.reject(new Error('offline'))
    else japanese.resolve(resource('ja'))
    assert.equal(await oldChange, false)
    assert.equal(instance.language, 'fr')
    assert.deepEqual(cached, ['fr'])
  }
})

test('normalizes browser and saved language codes before loading', async () => {
  for (const [input, expected] of [
    ['zh-Hant-HK', 'zhTW'],
    ['fr-FR', 'fr'],
    ['unsupported', 'en'],
  ] as Array<[string, InterfaceLanguageCode]>) {
    const { instance, requested, loader } = await setup()
    await loader.change(input)
    assert.equal(instance.language, expected)
    assert.deepEqual(requested, expected === 'en' ? ['en'] : ['en', expected])
  }
})
