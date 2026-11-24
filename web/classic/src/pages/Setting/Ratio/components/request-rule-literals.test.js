import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  buildRequestRuleExpr,
  splitBillingExprAndRequestRules,
  tryParseRequestRuleExpr,
} from './requestRuleExpr.js';

test('keeps unsupported equality literals in the original billing expression', () => {
  for (const value of ['["express"]', '{"speed":"express"}', '"123"', '" express "']) {
    const rule = `(param("options") == ${value} ? 2 : 1)`;
    const expr = `(tier("base", p * 1)) * ${rule}`;
    assert.equal(tryParseRequestRuleExpr(rule), null);
    assert.deepEqual(splitBillingExprAndRequestRules(expr), { billingExpr: expr, requestRuleExpr: '' });
  }
  for (const scalar of ['"express"', 'true', '1.5']) {
    const rule = `(param("options") == ${scalar} ? 2 : 1)`;
    assert.equal(buildRequestRuleExpr(tryParseRequestRuleExpr(rule)), rule);
  }
});
