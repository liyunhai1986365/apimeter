import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  ANNOUNCEMENT_CATEGORY_OPTIONS,
  ANNOUNCEMENT_TYPE_OPTIONS,
  filterAnnouncementsByCategory,
  getAnnouncementCategoryLabel,
} from './announcement-categories'

describe('announcement categories', () => {
  test('exposes only announcement-oriented creatable types', () => {
    assert.deepEqual(
      ANNOUNCEMENT_TYPE_OPTIONS.map((option) => option.value),
      [
        'product_update',
        'system_maintenance',
        'model_release',
        'pricing_update',
        'incident',
        'general',
      ]
    )
    assert.equal(getAnnouncementCategoryLabel('model_release'), 'Model Release')
  })

  test('filters announcements by publication category', () => {
    const announcements = [
      { id: 1, title: 'Platform notice', type: 'product_update' as const },
      { id: 2, title: 'New model added', type: 'model_release' as const },
    ]

    assert.deepEqual(
      filterAnnouncementsByCategory(announcements, 'model_release').map(
        (item) => item.title
      ),
      ['New model added']
    )

    assert.deepEqual(
      filterAnnouncementsByCategory(announcements, 'product_update').map(
        (item) => item.title
      ),
      ['Platform notice']
    )
  })

  test('exposes category tabs for the dashboard and dialog', () => {
    assert.deepEqual(
      ANNOUNCEMENT_CATEGORY_OPTIONS.map((option) => option.value),
      ['all', 'product_update', 'system_maintenance', 'model_release']
    )
  })
})
