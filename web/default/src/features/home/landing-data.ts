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
import type { LucideIcon } from 'lucide-react'
import {
  Activity,
  Braces,
  Building2,
  CircleDollarSign,
  CircleGauge,
  Cloud,
  KeyRound,
  LayoutGrid,
  LineChart,
  Radar,
  Route,
  ShieldCheck,
  SlidersHorizontal,
  WalletCards,
} from 'lucide-react'

export interface LogoRailProvider {
  id: string
  name: string
  iconName: string
}

export interface MetricItem {
  value: string
  labelKey: string
}

export interface ControlSignal {
  labelKey: string
  value: string
  icon: LucideIcon
}

export interface Snippet {
  id: 'curl' | 'python' | 'javascript'
  label: string
  code: string
}

export interface CopyCard {
  titleKey: string
  descriptionKey: string
  icon: LucideIcon
  accentClassName: string
}

export interface StepCard {
  titleKey: string
  descriptionKey: string
  icon: LucideIcon
}

export const LOGO_RAIL_PROVIDERS: LogoRailProvider[] = [
  { id: 'openai', name: 'OpenAI', iconName: 'OpenAI.Color' },
  { id: 'claude', name: 'Claude', iconName: 'Claude.Color' },
  { id: 'gemini', name: 'Gemini', iconName: 'Gemini.Color' },
  { id: 'deepseek', name: 'DeepSeek', iconName: 'DeepSeek.Color' },
  { id: 'qwen', name: 'Qwen', iconName: 'Qwen.Color' },
  { id: 'grok', name: 'Grok', iconName: 'Grok.Color' },
  { id: 'doubao', name: 'Doubao', iconName: 'Doubao.Color' },
  { id: 'moonshot', name: 'Moonshot', iconName: 'Moonshot.Color' },
  { id: 'perplexity', name: 'Perplexity', iconName: 'Perplexity.Color' },
  { id: 'mistral', name: 'Mistral', iconName: 'Mistral.Color' },
  { id: 'azure', name: 'Azure', iconName: 'Azure.Color' },
  { id: 'bedrock', name: 'Bedrock', iconName: 'Bedrock.Color' },
]

export const METRICS: MetricItem[] = [
  { value: '40+', labelKey: 'upstream providers' },
  { value: '100+', labelKey: 'billable models' },
  { value: '4', labelKey: 'compatible API families' },
  { value: '24/7', labelKey: 'routing observability' },
]

export const CONTROL_SIGNALS: ControlSignal[] = [
  { labelKey: 'Selected group', value: 'auto', icon: LayoutGrid },
  { labelKey: 'Healthy channels', value: '18 / 20', icon: Activity },
  { labelKey: 'p95 latency', value: '612ms', icon: CircleGauge },
]

export const CODE_SNIPPETS: Snippet[] = [
  {
    id: 'curl',
    label: 'curl',
    code: `curl https://your-domain.com/v1/chat/completions \\
  -H "Authorization: Bearer $API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "claude-opus-4.7",
    "messages": [{"role": "user", "content": "Ship it"}]
  }'`,
  },
  {
    id: 'python',
    label: 'Python',
    code: `from openai import OpenAI

client = OpenAI(
    base_url="https://your-domain.com/v1",
    api_key="ai_gateway_sk_..."
)

response = client.chat.completions.create(
    model="gpt-5.5",
    messages=[{"role": "user", "content": "Ship it"}],
)`,
  },
  {
    id: 'javascript',
    label: 'JavaScript',
    code: `import OpenAI from "openai";

const client = new OpenAI({
  baseURL: "https://your-domain.com/v1",
  apiKey: process.env.API_KEY,
});

const response = await client.chat.completions.create({
  model: "gemini-3.1-pro",
  messages: [{ role: "user", content: "Ship it" }],
});`,
  },
]

export const COPY_CARDS: CopyCard[] = [
  {
    titleKey: 'One key for every model',
    descriptionKey:
      'Issue a single API key, assign groups, and keep every client on the same contract.',
    icon: KeyRound,
    accentClassName: 'text-blue-700 bg-blue-50 dark:bg-blue-950 dark:text-blue-300',
  },
  {
    titleKey: 'Policy-driven routing',
    descriptionKey:
      'Route by priority, weight, model, and channel health without changing client code.',
    icon: Route,
    accentClassName:
      'text-emerald-700 bg-emerald-50 dark:bg-emerald-950 dark:text-emerald-300',
  },
  {
    titleKey: 'Transparent cost control',
    descriptionKey:
      'Track quota, recharge, model pricing, and usage from one billing surface.',
    icon: CircleDollarSign,
    accentClassName:
      'text-orange-700 bg-orange-50 dark:bg-orange-950 dark:text-orange-300',
  },
  {
    titleKey: 'Operations-grade monitoring',
    descriptionKey:
      'Inspect request logs, latency, errors, and per-channel routing health live.',
    icon: Radar,
    accentClassName:
      'text-sky-700 bg-sky-50 dark:bg-sky-950 dark:text-sky-300',
  },
]

export const STEP_CARDS: StepCard[] = [
  {
    titleKey: 'Connect providers',
    descriptionKey:
      'Add upstream accounts and map model names once inside the admin console.',
    icon: Cloud,
  },
  {
    titleKey: 'Ship one API contract',
    descriptionKey:
      'Point apps at OpenAI-compatible, Claude-compatible, or Gemini-compatible routes.',
    icon: Braces,
  },
  {
    titleKey: 'Observe every request',
    descriptionKey:
      'Use logs, rankings, and billing data to tune routing decisions continuously.',
    icon: LineChart,
  },
]

export const TRUST_CARDS: CopyCard[] = [
  {
    titleKey: 'Provider governance',
    descriptionKey:
      'Centralize upstream credentials, routing priorities, and availability rules.',
    icon: SlidersHorizontal,
    accentClassName:
      'text-slate-700 bg-slate-100 dark:bg-slate-900 dark:text-slate-200',
  },
  {
    titleKey: 'Billing integrity',
    descriptionKey:
      'Usage logs, quota records, and pricing rules stay aligned across users and teams.',
    icon: WalletCards,
    accentClassName:
      'text-emerald-700 bg-emerald-50 dark:bg-emerald-950 dark:text-emerald-300',
  },
  {
    titleKey: 'Enterprise access',
    descriptionKey:
      'Support admin control, reseller scenarios, team permissions, and protected routes.',
    icon: Building2,
    accentClassName:
      'text-blue-700 bg-blue-50 dark:bg-blue-950 dark:text-blue-300',
  },
  {
    titleKey: 'Security baseline',
    descriptionKey:
      'JWT auth, passkeys, OAuth, rate limits, and audit-friendly visibility.',
    icon: ShieldCheck,
    accentClassName:
      'text-orange-700 bg-orange-50 dark:bg-orange-950 dark:text-orange-300',
  },
]
