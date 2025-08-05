package compression

import (
	"compress/flate"
	"io"
)

const (
	EncodingDeflate = "deflate"
)

// DeflateCompressor implements the compression handler for deflate encoding.
type DeflateCompressor struct{}

// Compress the reader content with deflate encoding.
func (dc DeflateCompressor) Compress(w io.Writer, src io.Reader) (int64, error) {
	zw, err := flate.NewWriter(w, flate.DefaultCompression)
	if err != nil {
		return 0, err
	}

	size, err := io.Copy(zw, src)
	_ = zw.Close()

	return size, err
}

// Decompress the reader content with deflate encoding.
func (dc DeflateCompressor) Decompress(reader io.ReadCloser) (io.ReadCloser, error) {
	compressionReader := flate.NewReader(reader)

	return readCloserWrapper{
		CompressionReader: compressionReader,
		OriginalReader:    reader,
	}, nil
}
