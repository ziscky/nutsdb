package nutsdb

import (
	"bufio"
	"io"
	"os"
)

// fileRecovery use bufio.Reader to read entry
type fileRecovery struct {
	fd     *os.File
	reader *bufio.Reader
	size   int64
}

func newFileRecovery(path string, bufSize int) (fr *fileRecovery, err error) {
	fd, err := os.OpenFile(path, os.O_RDWR, os.ModePerm)
	if err != nil {
		return nil, err
	}
	bufSize = calBufferSize(bufSize)
	fileInfo, err := fd.Stat()
	if err != nil {
		return nil, err
	}
	return &fileRecovery{
		fd:     fd,
		reader: bufio.NewReaderSize(fd, bufSize),
		size:   fileInfo.Size(),
	}, nil
}

// readEntry will read an entry from disk.
func (fr *fileRecovery) readEntry(off int64) (e *entry, err error) {
	var size int64 = maxEntryHeaderSize
	// Since maxEntryHeaderSize may be larger than the actual Header, it needs to be calculated
	if off+size > fr.size {
		size = fr.size - off
	}

	buf := make([]byte, size)
	_, err = fr.fd.Seek(off, 0)
	if err != nil {
		return nil, err
	}

	_, err = fr.fd.Read(buf)
	if err != nil {
		return nil, err
	}

	e = new(entry)
	headerSize, err := e.parseMeta(buf)
	if err != nil {
		return nil, err
	}

	if e.isZero() {
		return nil, nil
	}

	headerBuf := buf[:headerSize]
	remainingBuf := buf[headerSize:]

	payloadSize := e.Meta.payloadSize()
	dataBuf := make([]byte, payloadSize)
	excessSize := size - headerSize

	if payloadSize <= excessSize {
		copy(dataBuf, remainingBuf[:payloadSize])
	} else {
		copy(dataBuf, remainingBuf)
		_, err := fr.fd.Read(dataBuf[excessSize:])
		if err != nil {
			return nil, err
		}
	}

	err = e.parsePayload(dataBuf)
	if err != nil {
		return nil, err
	}

	crc := e.getCrc(headerBuf)
	if crc != e.Meta.Crc {
		return nil, ErrCrc
	}

	return e, nil
}

func (fr *fileRecovery) readBucket() (b *bucket, err error) {
	buf := make([]byte, bucketMetaSize)
	_, err = io.ReadFull(fr.reader, buf)
	if err != nil {
		return nil, err
	}
	meta := new(bucketMeta)
	meta.decode(buf)
	bucket := new(bucket)
	bucket.Meta = meta
	dataBuf := make([]byte, meta.Size)
	_, err = io.ReadFull(fr.reader, dataBuf)
	if err != nil {
		return nil, err
	}
	err = bucket.decode(dataBuf)
	if err != nil {
		return nil, err
	}

	if bucket.getCRC(buf, dataBuf) != bucket.Meta.Crc {
		return nil, ErrBucketCrcInvalid
	}

	return bucket, nil
}

// calBufferSize calculates the buffer size of bufio.Reader
// if the size < 4 * kb, use 4 * kb as the size of buffer in bufio.Reader
// if the size > 4 * kb, use the nearly blockSize buffer as the size of buffer in bufio.Reader
func calBufferSize(size int) int {
	blockSize := 4 * kb
	if size < blockSize {
		return blockSize
	}
	hasRest := (size%blockSize == 0)
	if hasRest {
		return (size/blockSize + 1) * blockSize
	}
	return size
}

func (fr *fileRecovery) release() error {
	return fr.fd.Close()
}
