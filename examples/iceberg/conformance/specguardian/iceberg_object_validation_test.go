//go:build specguardian

package specguardian_test

import (
	"bytes"
	"path"
	"strings"
	"testing"

	"github.com/apache/arrow-go/v18/parquet/file"
	iceberggo "github.com/apache/iceberg-go"
	"github.com/apache/iceberg-go/table"
	"github.com/stretchr/testify/require"
)

// validateNewObjects checks files left by the action, including uncommitted files.
func validateNewObjects(t *testing.T, objects map[string][]byte) {
	t.Helper()
	for key, contents := range objects {
		switch {
		case strings.HasSuffix(key, ".parquet"):
			reader, err := file.NewParquetReader(bytes.NewReader(contents))
			require.NoError(t, err, "invalid data file %s", key)
			require.NoError(t, reader.Close())
		case strings.HasSuffix(key, ".metadata.json"):
			_, err := table.ParseMetadataBytes(contents)
			require.NoError(t, err, "invalid metadata %s", key)
		case strings.HasSuffix(key, ".avro") && strings.HasPrefix(path.Base(key), "snap-"):
			_, err := iceberggo.ReadManifestList(bytes.NewReader(contents))
			require.NoError(t, err, "invalid manifest list %s", key)
		case strings.HasSuffix(key, ".avro"):
			require.NotEmpty(t, contents, "empty manifest %s", key)
		default:
			t.Errorf("unexpected S3 object %s", key)
		}
	}
}
