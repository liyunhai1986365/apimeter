package common

import (
	"strings"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/tidwall/gjson"
)

// ImageResponseCount counts completed OpenAI image entries in either array or
// object-shaped data. Multiple representations on one entry describe one image;
// separate entries count separately even when their formats differ.
func ImageResponseCount(body []byte) int64 {
	data := gjson.GetBytes(body, "data")
	if !data.IsArray() && !data.IsObject() {
		return 0
	}
	var count int64
	for _, item := range data.Array() {
		for _, field := range []string{"url", "b64_json", "base64", "image_base64", "imageBase64"} {
			value := item.Get(field)
			if value.Type == gjson.String && strings.TrimSpace(value.String()) != "" {
				count++
				break
			}
		}
	}
	return count
}

func (info *RelayInfo) SetImageBillingResponse(body []byte) {
	if info == nil || len(body) == 0 {
		return
	}
	if info.BillingRequestInput == nil {
		info.BillingRequestInput = &billingexpr.RequestInput{}
	}
	info.BillingRequestInput.ResponseBody = append(info.BillingRequestInput.ResponseBody[:0], body...)
}

func (info *RelayInfo) RequestedImageCount() int {
	if request, ok := info.Request.(*dto.ImageRequest); ok && request.N != nil && *request.N > 0 {
		return int(*request.N)
	}
	if count := info.PriceData.OtherRatios["n"]; count > 0 {
		return int(count)
	}
	return 1
}

// UpdateImageCount adapts upstream image quantities to local fixed-price
// billing. Token usage is already aggregated, so it must not be multiplied.
func (info *RelayInfo) UpdateImageCount(count int64) {
	if info == nil || count <= 0 {
		return
	}
	quantity := int(count)
	info.BillingImageCount = &quantity
	if info.PriceData.UsePrice {
		info.PriceData.AddOtherRatio("n", float64(count))
	}
}
