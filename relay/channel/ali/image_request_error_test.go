package ali

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestAliInvalidImageParametersAreRequestError(t *testing.T) {
	request := dto.ImageRequest{
		Model:  "wan2.6-t2i",
		Prompt: "draw a cat",
		Extra: map[string]json.RawMessage{
			"parameters": json.RawMessage(`"not-an-object"`),
		},
	}

	_, err := oaiImage2AliImageRequest(&relaycommon.RelayInfo{}, request, false)
	require.Error(t, err)
	require.True(t, types.IsRequestError(err))
}
