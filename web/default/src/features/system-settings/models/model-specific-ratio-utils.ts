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
import { safeJsonParse } from '../utils/json-parser'

export type GroupModelRatioRule = {
  group: string
  model: string
  ratio: number
}

export type UserGroupModelRatioRule = GroupModelRatioRule & {
  userGroup: string
}

export type UserGroupRatioRule = {
  userGroup: string
  group: string
  ratio: number
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function isValidRatio(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
}

function compareGroupRules(
  left: GroupModelRatioRule,
  right: GroupModelRatioRule
) {
  if (left.group !== right.group) return left.group.localeCompare(right.group)
  return left.model.localeCompare(right.model)
}

function compareUserGroupRules(
  left: UserGroupModelRatioRule,
  right: UserGroupModelRatioRule
) {
  if (left.userGroup !== right.userGroup) {
    return left.userGroup.localeCompare(right.userGroup)
  }
  return compareGroupRules(left, right)
}

function compareUserGroupRatioRules(
  left: UserGroupRatioRule,
  right: UserGroupRatioRule
) {
  if (left.userGroup !== right.userGroup) {
    return left.userGroup.localeCompare(right.userGroup)
  }
  return left.group.localeCompare(right.group)
}

export function parseGroupModelRatioRules(
  value: string
): GroupModelRatioRule[] {
  const parsed = safeJsonParse<Record<string, unknown>>(value, {
    fallback: {},
    silent: true,
  })
  const rules: GroupModelRatioRule[] = []

  for (const [rawGroup, rawModels] of Object.entries(parsed)) {
    const group = rawGroup.trim()
    if (!group || !isRecord(rawModels)) continue
    for (const [rawModel, ratio] of Object.entries(rawModels)) {
      const model = rawModel.trim()
      if (!model || !isValidRatio(ratio)) continue
      rules.push({ group, model, ratio })
    }
  }

  return rules.sort(compareGroupRules)
}

export function parseUserGroupModelRatioRules(
  value: string
): UserGroupModelRatioRule[] {
  const parsed = safeJsonParse<Record<string, unknown>>(value, {
    fallback: {},
    silent: true,
  })
  const rules: UserGroupModelRatioRule[] = []

  for (const [rawUserGroup, rawGroups] of Object.entries(parsed)) {
    const userGroup = rawUserGroup.trim()
    if (!userGroup || !isRecord(rawGroups)) continue
    for (const [rawGroup, rawModels] of Object.entries(rawGroups)) {
      const group = rawGroup.trim()
      if (!group || !isRecord(rawModels)) continue
      for (const [rawModel, ratio] of Object.entries(rawModels)) {
        const model = rawModel.trim()
        if (!model || !isValidRatio(ratio)) continue
        rules.push({ userGroup, group, model, ratio })
      }
    }
  }

  return rules.sort(compareUserGroupRules)
}

export function parseUserGroupRatioRules(value: string): UserGroupRatioRule[] {
  const parsed = safeJsonParse<Record<string, unknown>>(value, {
    fallback: {},
    silent: true,
  })
  const rules: UserGroupRatioRule[] = []

  for (const [rawUserGroup, rawGroups] of Object.entries(parsed)) {
    const userGroup = rawUserGroup.trim()
    if (!userGroup || !isRecord(rawGroups)) continue
    for (const [rawGroup, ratio] of Object.entries(rawGroups)) {
      const group = rawGroup.trim()
      if (!group || !isValidRatio(ratio)) continue
      rules.push({ userGroup, group, ratio })
    }
  }

  return rules.sort(compareUserGroupRatioRules)
}

export function serializeGroupModelRatioRules(
  rules: GroupModelRatioRule[]
): string {
  const result = Object.create(null) as Record<string, Record<string, number>>
  for (const rule of [...rules].sort(compareGroupRules)) {
    const group = rule.group.trim()
    const model = rule.model.trim()
    if (!group || !model || !isValidRatio(rule.ratio)) continue
    if (!result[group]) {
      result[group] = Object.create(null) as Record<string, number>
    }
    result[group][model] = rule.ratio
  }
  return JSON.stringify(result, null, 2)
}

export function serializeUserGroupModelRatioRules(
  rules: UserGroupModelRatioRule[]
): string {
  const result = Object.create(null) as Record<
    string,
    Record<string, Record<string, number>>
  >
  for (const rule of [...rules].sort(compareUserGroupRules)) {
    const userGroup = rule.userGroup.trim()
    const group = rule.group.trim()
    const model = rule.model.trim()
    if (!userGroup || !group || !model || !isValidRatio(rule.ratio)) continue
    if (!result[userGroup]) {
      result[userGroup] = Object.create(null) as Record<
        string,
        Record<string, number>
      >
    }
    if (!result[userGroup][group]) {
      result[userGroup][group] = Object.create(null) as Record<string, number>
    }
    result[userGroup][group][model] = rule.ratio
  }
  return JSON.stringify(result, null, 2)
}

export function serializeUserGroupRatioRules(
  rules: UserGroupRatioRule[]
): string {
  const result = Object.create(null) as Record<string, Record<string, number>>
  for (const rule of [...rules].sort(compareUserGroupRatioRules)) {
    const userGroup = rule.userGroup.trim()
    const group = rule.group.trim()
    if (!userGroup || !group || !isValidRatio(rule.ratio)) continue
    if (!result[userGroup]) {
      result[userGroup] = Object.create(null) as Record<string, number>
    }
    result[userGroup][group] = rule.ratio
  }
  return JSON.stringify(result, null, 2)
}

export function buildConfiguredModelNameOptions(
  ...configs: string[]
): string[] {
  const names = new Set<string>()
  for (const config of configs) {
    const parsed = safeJsonParse<Record<string, unknown>>(config, {
      fallback: {},
      silent: true,
    })
    for (const rawName of Object.keys(parsed)) {
      const name = rawName.trim()
      if (name) names.add(name)
    }
  }
  return [...names].sort((left, right) => left.localeCompare(right))
}
