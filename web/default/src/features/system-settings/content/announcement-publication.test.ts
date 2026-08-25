import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  planAnnouncementPublication,
  type Announcement,
  type AnnouncementDraft,
} from './announcement-publication'

const existing: Announcement = {
  id: 1,
  title: 'Existing announcement',
  content: 'Existing content',
  publishDate: '2026-07-28T00:00:00.000Z',
  type: 'general',
  extra: '',
  audience: 'all',
}

const draft: AnnouncementDraft = {
  title: 'Email broadcast',
  content: 'Email content',
  publishDate: '2026-07-28T01:00:00.000Z',
  type: 'product_update',
  extra: '',
  audience: 'main_site',
}

describe('announcement publication plan', () => {
  test('does not save a new email-only announcement to the frontend list', () => {
    const announcements = [existing]
    const result = planAnnouncementPublication({
      announcements,
      draft,
      displayOnFrontend: false,
    })

    assert.equal(result.shouldSave, false)
    assert.equal(result.announcements, announcements)
  })

  test('removes an edited announcement when changing it to email only', () => {
    const result = planAnnouncementPublication({
      announcements: [existing],
      draft,
      editingId: existing.id,
      displayOnFrontend: false,
    })

    assert.equal(result.shouldSave, true)
    assert.deepEqual(result.announcements, [])
  })

  test('adds a frontend announcement with the next available id', () => {
    const result = planAnnouncementPublication({
      announcements: [existing],
      draft,
      displayOnFrontend: true,
    })

    assert.equal(result.shouldSave, true)
    assert.deepEqual(result.announcements, [existing, { id: 2, ...draft }])
  })
})
