package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	"github.com/stretchr/testify/require"
)

func TestGetAdaptorReturnsConfigurableImageAdaptor(t *testing.T) {
	adaptor := GetAdaptor(constant.APITypeConfigurable)
	require.IsType(t, &configurable.ImageAdaptor{}, adaptor)
}
