package model

import (
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBillingOptionValidationPreservesPrices(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "options.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	oldDB, oldOptions := DB, common.OptionMap
	oldPrices := ratio_setting.ModelPrice2JSONString()
	oldExpr, err := common.Marshal(billing_setting.GetBillingExprCopy())
	require.NoError(t, err)
	DB, common.OptionMap = db, map[string]string{}
	const exprKey = "billing_setting.billing_expr"
	t.Cleanup(func() {
		DB, common.OptionMap = oldDB, oldOptions
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrices))
		billing_setting.UpdateBillingField(billing_setting.BillingExprField, string(oldExpr))
	})
	const prices = `{"paid":0.0017,"free":0}`
	const expression = `param("service_tier") == "fast" ? tier("fast",p*4+c*14) : tier("base",p*2+c*10)`
	exprs, err := common.Marshal(map[string]string{"paid": expression})
	require.NoError(t, err)
	valid := map[string]string{"ModelPrice": prices, exprKey: string(exprs)}
	require.NoError(t, UpdateOptionsBulk(valid))

	checkRuntime := func() {
		t.Helper()
		require.Equal(t, valid, common.OptionMap)
		require.Equal(t, map[string]float64{"paid": 0.0017, "free": 0}, ratio_setting.GetModelPriceCopy())
		require.Equal(t, map[string]string{"paid": expression}, billing_setting.GetBillingExprCopy())
	}
	for _, tc := range []struct{ key, raw string }{
		{"ModelPrice", `{"new":0,"paid":"bad"}`}, {"ModelPrice", `{"paid":null}`},
		{"ModelPrice", `{"paid":-1}`}, {"ModelPrice", `{"paid":1e999}`}, {"ModelPrice", `null`},
		{exprKey, `{"paid":"tier(\"broken\", p + )","free":"0"}`}, {exprKey, `{"paid":null}`},
		{exprKey, `{"paid":"p / c"}`}, {exprKey, `[]`}, {exprKey, `null`},
	} {
		require.Error(t, UpdateOption(tc.key, tc.raw))
		updates := map[string]string{"ModelPrice": "{}", exprKey: "{}"}
		updates[tc.key] = tc.raw
		require.Error(t, UpdateOptionsBulk(updates))
		for key, value := range valid {
			var stored Option
			require.NoError(t, db.First(&stored, &Option{Key: key}).Error)
			require.Equal(t, value, stored.Value)
		}
		checkRuntime()
	}

	// Simulate rows written by an older gateway. A refresh must keep the
	// active values, including the option values shown to administrators.
	require.NoError(t, db.Save(&Option{Key: "ModelPrice", Value: `{"paid":"bad"}`}).Error)
	require.NoError(t, db.Save(&Option{Key: exprKey, Value: `{"paid":"p +"}`}).Error)
	loadOptionsFromDatabase()
	checkRuntime()
	require.NoError(t, UpdateOptionsBulk(map[string]string{"ModelPrice": "{}", exprKey: "{}"}))
	require.Empty(t, ratio_setting.GetModelPriceCopy())
	require.Empty(t, billing_setting.GetBillingExprCopy())
}
