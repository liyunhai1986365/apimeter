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

export function removeModelFromChannelModels(
  models: string,
  modelName: string
): string {
  const target = modelName.trim()
  if (!target) return models

  const seen = new Set<string>()
  const nextModels = models
    .split(',')
    .map((model) => model.trim())
    .filter((model) => model.length > 0)
    .filter((model) => {
      if (model === target) return false
      if (seen.has(model)) return false
      seen.add(model)
      return true
    })

  return nextModels.join(',')
}
