package common

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type failingMultipartBodyStorage struct{}

func (failingMultipartBodyStorage) Read([]byte) (int, error)       { return 0, io.EOF }
func (failingMultipartBodyStorage) Seek(int64, int) (int64, error) { return 0, nil }
func (failingMultipartBodyStorage) Close() error                   { return nil }
func (failingMultipartBodyStorage) Bytes() ([]byte, error) {
	return nil, errors.New("disk read failed")
}
func (failingMultipartBodyStorage) Size() int64  { return 1 }
func (failingMultipartBodyStorage) IsDisk() bool { return true }

func TestParseMultipartFormReusableClassifiesOnlyRequestFormatErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("malformed multipart is request error", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/upload", nil)
		c.Request.Header.Set("Content-Type", "multipart/form-data")
		storage, err := CreateBodyStorage([]byte("not-multipart"))
		require.NoError(t, err)
		defer storage.Close()
		c.Set(KeyBodyStorage, storage)

		_, err = ParseMultipartFormReusable(c)
		require.Error(t, err)
		require.True(t, IsMultipartRequestError(err))
	})

	t.Run("storage read failure remains server error", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/upload", nil)
		c.Request.Header.Set("Content-Type", "multipart/form-data; boundary=test")
		c.Set(KeyBodyStorage, failingMultipartBodyStorage{})

		_, err := ParseMultipartFormReusable(c)
		require.Error(t, err)
		require.False(t, IsMultipartRequestError(err))
	})
}
