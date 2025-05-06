package compression

import (
	"compress/zlib"
	"io"
)

const (
	EncodingDeflate = "deflate"
)

// DeflateCompressor implements the compression handler for deflate encoding.
type DeflateCompressor struct{}

// Compress the reader content with gzip encoding.
func (dc DeflateCompressor) Compress(w io.Writer, src io.Reader) (int64, error) {
	zw := zlib.NewWriter(w)

	size, err := io.Copy(zw, src)
	_ = zw.Close()

	return size, err
}

// Decompress the reader content with gzip encoding.
func (dc DeflateCompressor) Decompress(reader io.ReadCloser) (io.ReadCloser, error) {
	compressionReader, err := zlib.NewReader(reader)
	if err != nil {
		return nil, err
	}

	return readCloserWrapper{
		CompressionReader: compressionReader,
		OriginalReader:    reader,
	}, nil
}
