package relay

import (
	"fmt"
	"math"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type outboundImageCounter interface {
	OutboundImageCount([]byte) (*uint, error)
}

// Reconcile Imagen's final sampleCount after channel overrides, before sending.
// Expressions retain their frozen client request; actual output counts still
// take precedence over this estimate at settlement.
func prepareOutboundImageCount(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ImageRequest, adaptor any, body []byte) *types.NewAPIError {
	counter, ok := adaptor.(outboundImageCounter)
	if !ok {
		return nil
	}
	count, err := counter.OutboundImageCount(body)
	if err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	if count == nil {
		return nil
	}
	if *count == 0 || uint64(*count) > uint64(math.MaxInt) {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid outbound image count"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	request.N = count
	info.OutboundImageCount = count
	if !info.PriceData.UsePrice || info.TieredBillingSnapshot != nil {
		return nil
	}
	info.PriceData.AddOtherRatio("n", float64(*count))
	if info.PriceData.FreeModel {
		return nil
	}
	unit := info.PriceData.FixedQuotaPerUnit
	if unit == nil {
		unit = &common.QuotaPerUnit
	}
	quota, err := common.QuotaFromFloatStrict(info.PriceData.ApplyOtherRatiosToFloat(info.PriceData.ModelPrice * (*unit) * info.PriceData.GroupRatioInfo.GroupRatio))
	if err != nil {
		return types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithSkipRetry())
	}
	info.PriceData.QuotaToPreConsume = quota
	if isLocalImageAsyncWorker(c) {
		// A local task already owns its initial pre-consumption. Publish the
		// increase and its task CAS in the same transaction.
		if info.Billing == nil {
			return types.NewError(fmt.Errorf("missing local image billing session"), types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
		}
		var publish func()
		if err := info.Billing.ReserveWithCommit(quota, func(tx *gorm.DB, reservedQuota int) error {
			var err error
			publish, err = syncLocalImageAsyncReservation(c, info, tx, reservedQuota)
			return err
		}); err != nil {
			return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
		}
		publish()
		info.FinalPreConsumedQuota = info.Billing.GetPreConsumedQuota()
		return nil
	}
	if info.Billing == nil {
		if apiErr := service.PreConsumeBilling(c, quota, info); apiErr != nil {
			return apiErr
		}
	} else if err := info.Billing.Reserve(quota); err != nil {
		return types.NewError(err, types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}
	info.FinalPreConsumedQuota = info.Billing.GetPreConsumedQuota()
	return nil
}
