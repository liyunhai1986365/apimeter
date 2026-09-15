package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSeedanceResponseValidation(t *testing.T) {
	for _, body := range []string{
		`{"id":"cgt-owner","status":"running"}`,
		`{"task":{"id":"cgt-owner","status":"preparing"}}`,
		`{"data":{"task_id":"cgt-owner","status":"completed"}}`,
		`{"data":{"data":{"task":{"id":"cgt-owner","status":"expired"}}}}`,
	} {
		require.NoError(t, ValidateSeedanceTaskIdentity([]byte(body), &TaskInfo{TaskID: "cgt-owner"}, "cgt-owner", ""))
		require.NoError(t, ValidateSeedanceTaskStatus([]byte(body)))
	}
	for _, body := range []string{
		`{"task":{"status":"succeeded"}}`,
		`{"task":{"id":null}}`, `{"task":{"id":123}}`, `{"task":{"id":""}}`,
		`{"data":{"task_id":"cgt-other"}}`,
	} {
		require.Error(t, ValidateSeedanceTaskIdentity([]byte(body), nil, "cgt-owner", ""), body)
	}
	for _, body := range []string{
		`{}`, `{"task":{"status":null}}`, `{"data":{"status":false}}`,
		`{"status":[]}`, `{"status":{}}`, `{"status":""}`, `{"status":"unexpected"}`,
	} {
		require.Error(t, ValidateSeedanceTaskStatus([]byte(body)), body)
	}
}
