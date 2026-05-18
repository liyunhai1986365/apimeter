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

import { strict as assert } from 'node:assert'
import { normalizePagedData } from '../src/lib/paged-response.ts'

assert.deepEqual(normalizePagedData(undefined), { items: [], total: 0 })
assert.deepEqual(normalizePagedData({ success: false }), {
  items: [],
  total: 0,
})
assert.deepEqual(
  normalizePagedData({
    success: true,
    data: { items: [{ id: 1 }], total: 3, page: 1, page_size: 20 },
  }),
  { items: [{ id: 1 }], total: 3, page: 1, page_size: 20 }
)

console.log('normalizePagedData handles missing and failed paged responses')
