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
			"index.html":  &fstest.MapFile{Data: []byte("index")},
			"robots.txt":  &fstest.MapFile{Data: []byte("stale robots")},
			"sitemap.xml": &fstest.MapFile{Data: []byte("stale sitemap")},
			"app.js":      &fstest.MapFile{Data: []byte("app")},
		}),
	}

	require.False(t, fs.Exists("/", "/index.html"))
	require.False(t, fs.Exists("/", "/robots.txt"))
	require.False(t, fs.Exists("/", "/sitemap.xml"))

	_, err := fs.Open("index.html")
	require.True(t, errors.Is(err, os.ErrNotExist))

	require.True(t, fs.Exists("/", "/app.js"))
}
