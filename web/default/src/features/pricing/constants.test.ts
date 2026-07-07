import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { MODEL_CATEGORIES, getModelCategoryIconStyles } from './constants'

describe('pricing category metadata', () => {
  test('provides icon styling for every pricing category tab', () => {
    const styles = getModelCategoryIconStyles()

    for (const category of Object.values(MODEL_CATEGORIES)) {
      assert.ok(styles[category].icon)
      assert.match(styles[category].className, /text-/)
    }
  })
})
