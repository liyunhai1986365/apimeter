import { taskActionMapper } from './mappers'
import type { TaskLog, TaskLogProperties } from '../types'

type TranslateFn = (value: string) => string

function parseProperties(properties: TaskLog['properties']): TaskLogProperties {
  if (!properties) return {}
  if (typeof properties === 'string') {
    try {
      const parsed: unknown = JSON.parse(properties)
      return parsed && typeof parsed === 'object'
        ? (parsed as TaskLogProperties)
        : {}
    } catch {
      return {}
    }
  }
  return properties
}

function normalizeModelName(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function isImageTaskModel(modelName: string): boolean {
  const normalized = modelName.toLowerCase()
  if (/^gpt-image-\d+/.test(normalized)) return true
  return /^gemini-[\w.-]+-image(?:-[\w.-]+)?$/.test(normalized)
}

export function getTaskLogImageModelName(log: TaskLog): string {
  const properties = parseProperties(log.properties)
  const originModelName = normalizeModelName(properties.origin_model_name)
  const upstreamModelName = normalizeModelName(properties.upstream_model_name)

  if (isImageTaskModel(originModelName)) return originModelName
  if (isImageTaskModel(upstreamModelName)) return upstreamModelName
  return ''
}

export function buildTaskLogSubtitle(log: TaskLog, t: TranslateFn): string {
  const imageModelName = getTaskLogImageModelName(log)
  if (imageModelName) return imageModelName

  return `${t(log.platform)} · ${t(taskActionMapper.getLabel(log.action))}`
}
