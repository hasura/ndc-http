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
	fw, err := flate.NewWriter(w, flate.DefaultCompression)
	if err != nil {
		return 0, err
	}

	size, err := io.Copy(fw, src)
	if err != nil {
		return 0, err
	}
	if err := fw.Close(); err != nil {
		return 0, err
	}

	return size, nil
}

// Decompress the reader content with deflate encoding.
func (dc DeflateCompressor) Decompress(reader io.ReadCloser) (io.ReadCloser, error) {
	compressionReader := flate.NewReader(reader)

	return readCloserWrapper{
		CompressionReader: compressionReader,
		OriginalReader:    reader,
	}, nil
}
