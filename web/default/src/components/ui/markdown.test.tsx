import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { renderToStaticMarkup } from 'react-dom/server'
import { Markdown } from './markdown'

describe('Markdown', () => {
  test('renders heading nodes with concrete styles without typography plugin', () => {
    const html = renderToStaticMarkup(
      <Markdown>{'## 主题1\n正文111\n\n### 标题222\n正文222'}</Markdown>
    )

    assert.match(html, /<h2 class="[^"]*text-xl[^"]*"/)
    assert.match(html, /<h3 class="[^"]*text-lg[^"]*"/)
    assert.match(html, /<p class="[^"]*leading-relaxed[^"]*"/)
  })
})
