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
import {
  buildRequestRuleExpr,
  combineBillingExpr,
  splitBillingExprAndRequestRules,
  tryParseRequestRuleExpr,
  type RequestRuleGroup,
} from './billing-expr'
import { splitExprVersion } from './expr-syntax'
import {
  createDefaultVisualConfig,
  generateExprFromVisualConfig,
  tryParseVisualConfig,
  type VisualConfig,
} from './tier-expr'

export type PricingEditorState = {
  modelName: string
  billingExpr: string
  requestRuleExpr: string
  mode: 'visual' | 'raw'
  rawExpr: string
  visualConfig: VisualConfig | null
  ruleGroups: RequestRuleGroup[]
}

/** Hydration and mode changes must never publish a regenerated formula. */
export function createPricingEditorState(
  modelName: string,
  billingExpr: string,
  requestRuleExpr: string
): PricingEditorState {
  const config = billingExpr
    ? tryParseVisualConfig(billingExpr)
    : createDefaultVisualConfig()
  const groups = tryParseRequestRuleExpr(requestRuleExpr)
  return {
    modelName,
    billingExpr,
    requestRuleExpr,
    mode: config && groups ? 'visual' : 'raw',
    rawExpr: combineBillingExpr(billingExpr, requestRuleExpr),
    visualConfig: config,
    ruleGroups: groups || [],
  }
}

export function switchPricingEditorMode(
  state: PricingEditorState,
  mode: PricingEditorState['mode']
): PricingEditorState {
  if (mode === 'raw') return { ...state, mode }
  const parsed = createPricingEditorState(
    state.modelName,
    state.billingExpr,
    state.requestRuleExpr
  )
  if (parsed.mode !== 'visual') return state
  return { ...parsed, rawExpr: state.rawExpr }
}

export function editRawPricing(
  state: PricingEditorState,
  rawExpr: string
): PricingEditorState {
  return {
    ...state,
    ...splitBillingExprAndRequestRules(rawExpr),
    rawExpr,
    mode: 'raw',
  }
}

export function editVisualPricing(
  state: PricingEditorState,
  visualConfig: VisualConfig
): PricingEditorState {
  const { prefix } = splitExprVersion(state.billingExpr.trim())
  const billingExpr = prefix + generateExprFromVisualConfig(visualConfig)
  return {
    ...state,
    visualConfig,
    billingExpr,
    rawExpr: combineBillingExpr(billingExpr, state.requestRuleExpr),
  }
}

export function editPricingRules(
  state: PricingEditorState,
  ruleGroups: RequestRuleGroup[]
): PricingEditorState {
  const requestRuleExpr = buildRequestRuleExpr(ruleGroups)
  return {
    ...state,
    ruleGroups,
    requestRuleExpr,
    rawExpr: combineBillingExpr(state.billingExpr, requestRuleExpr),
  }
}
