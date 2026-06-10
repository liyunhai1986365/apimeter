package common

import (
	"errors"
	"net/http"
	"os"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func TestEmbedFileSystemSkipsIndexHTMLForDynamicRendering(t *testing.T) {
	fs := &embedFileSystem{
		FileSystem: http.FS(fstest.MapFS{
			"index.html": &fstest.MapFile{Data: []byte("index")},
			"app.js":     &fstest.MapFile{Data: []byte("app")},
		}),
	}

	require.False(t, fs.Exists("/", "/index.html"))

	_, err := fs.Open("index.html")
	require.True(t, errors.Is(err, os.ErrNotExist))

	require.True(t, fs.Exists("/", "/app.js"))
}
