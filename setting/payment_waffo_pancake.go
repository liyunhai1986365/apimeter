package setting

// Waffo Pancake hosted checkout configuration. Gateway is enabled once
// MerchantID + PrivateKey + ProductID are populated (no separate Enabled
// flag, matching Stripe / Creem). StoreID + ProductID are operator-bound
// via SaveWaffoPancakeConfig.
var (
	WaffoPancakeMerchantID string
	WaffoPancakePrivateKey string
	WaffoPancakeReturnURL  string
	WaffoPancakeUnitPrice  float64 = 1.0
	// Deprecated: Waffo Pancake now uses operation_setting.MinTopUp so the
	// payment-wide minimum has one source of truth.
	WaffoPancakeMinTopUp  int = 10
	WaffoPancakeStoreID   string
	WaffoPancakeProductID string
)
