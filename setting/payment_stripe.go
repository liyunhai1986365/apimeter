package setting

var StripeApiSecret = ""
var StripeWebhookSecret = ""
var StripePriceId = ""

// StripeEnabled controls whether new Stripe checkouts and automatic recharges
// can be started. It defaults to true to preserve existing installations where
// complete credentials implicitly enabled Stripe.
var StripeEnabled = true
var StripeUnitPrice = 8.0
var StripeMinTopUp = 1
var StripePromotionCodesEnabled = false
