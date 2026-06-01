package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestFetchNewAPISupplierSelfUsesExistingAccessToken(t *testing.T) {
	loginCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/login":
			loginCalled = true
			http.Error(w, "login should not be called", http.StatusInternalServerError)
		case "/api/user/self":
			require.Equal(t, "upstream-token", r.Header.Get("Authorization"))
			require.Equal(t, "42", r.Header.Get("New-Api-User"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":42,"username":"upstream","quota":100,"used_quota":5,"access_token":"upstream-token"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	supplier := &model.NewAPISupplier{
		BaseURL:        server.URL,
		AccessToken:    " upstream-token ",
		UpstreamUserID: 42,
	}

	self, accessToken, upstreamUserID, err := fetchNewAPISupplierSelf(context.Background(), server.Client(), supplier)

	require.NoError(t, err)
	require.False(t, loginCalled)
	require.Equal(t, "upstream", self.Username)
	require.Equal(t, "upstream-token", accessToken)
	require.Equal(t, 42, upstreamUserID)
}

func TestFetchNewAPISupplierSelfRequiresUserIDWithAccessToken(t *testing.T) {
	supplier := &model.NewAPISupplier{
		BaseURL:     "https://upstream.example.com",
		AccessToken: "upstream-token",
	}

	_, _, _, err := fetchNewAPISupplierSelf(context.Background(), http.DefaultClient, supplier)

	require.EqualError(t, err, "使用系统访问令牌时必须填写上游用户 ID")
}
