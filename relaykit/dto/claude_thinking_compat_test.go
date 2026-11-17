package dto

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestThinkingPreservesExplicitZeroAndUnknownFields(t *testing.T) {
	const raw = `{"type":"enabled","budget_tokens":0,"block_binding":{"prefix_mismatch_behavior":"drop_block"},"future_field":{"id":9007199254740993}}`
	var thinking Thinking
	require.NoError(t, kitutil.UnmarshalJsonStr(raw, &thinking))
	encoded, err := kitutil.Marshal(thinking)
	require.NoError(t, err)
	require.JSONEq(t, raw, string(encoded))
	require.Equal(t, "9007199254740993", gjson.GetBytes(encoded, "future_field.id").Raw)
	require.Equal(t, "0", gjson.GetBytes(encoded, "budget_tokens").Raw)

	// Reusing a DTO must not retain fields from an earlier request.
	require.NoError(t, kitutil.UnmarshalJsonStr(`{"type":"adaptive"}`, &thinking))
	encoded, err = kitutil.Marshal(thinking)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"adaptive"}`, string(encoded))
}

func TestThinkingExtensionsCannotOverrideKnownFields(t *testing.T) {
	thinking := Thinking{Type: "adaptive", Extra: map[string]json.RawMessage{
		"type": json.RawMessage(`"enabled"`), "budget_tokens": json.RawMessage(`99`),
		"future_field": json.RawMessage(`false`),
	}}
	encoded, err := kitutil.Marshal(thinking)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"adaptive","future_field":false}`, string(encoded))
}
