package service

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestImageTaskReservationWithoutAdditionalCharge(t *testing.T) {
	for _, mode := range []string{"user_owned_provider", "trusted_wallet", "unchanged_count", "decreased_count"} {
		t.Run(mode, func(t *testing.T) {
			info := &relaycommon.RelayInfo{}
			session := &BillingSession{relayInfo: info, funding: &WalletFunding{}, preConsumedQuota: 100}
			target := 100
			if mode == "user_owned_provider" {
				session.funding = &UserOwnedProviderFunding{}
				session.preConsumedQuota = 0
			} else if mode == "trusted_wallet" {
				session.trusted = true
				session.preConsumedQuota = 0
			} else if mode == "decreased_count" {
				target = 50
			}
			original := session.preConsumedQuota
			calls := 0
			// No accounts exist for this session: these paths must still run
			// the task publication without attempting any account mutation.
			require.NoError(t, session.ReserveWithCommit(target, func(tx *gorm.DB, reserved int) error {
				require.NotNil(t, tx)
				require.Equal(t, original, reserved)
				calls++
				return nil
			}))
			require.Equal(t, 1, calls)
			require.Equal(t, original, session.GetPreConsumedQuota())
			require.Equal(t, original, info.FinalPreConsumedQuota)
			require.Zero(t, session.extraReserved)
			require.Zero(t, session.tokenConsumed)
		})
	}
}
