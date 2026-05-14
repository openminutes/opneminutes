package media

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"hash/adler32"
	"io"
	"strconv"

	apperrors "openminutes/internal/errors"
)

const FileHeaderSize = 512

type Block struct {
	Hash     string `json:"hash"`
	Seq      int    `json:"seq"`
	Size     int64  `json:"size"`
	Checksum string `json:"checksum"`
}

func ReadHeader(reader io.Reader) (string, error) {
	header := make([]byte, FileHeaderSize)
	n, err := io.ReadFull(reader, header)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(header[:n]), nil
}

func ComputeBlocks(reader io.ReadSeeker, size int64, blockSize int64) ([]Block, error) {
	if blockSize <= 0 {
		return nil, apperrors.New(apperrors.KindRemote, "prepare response missing block_size")
	}
	if err := SeekStart(reader); err != nil {
		return nil, apperrors.Wrap(apperrors.KindFileSystem, err)
	}

	var blocks []Block
	buffer := make([]byte, blockSize)
	for seq := 0; ; seq++ {
		n, err := io.ReadFull(reader, buffer)
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
			return nil, apperrors.Wrap(apperrors.KindFileSystem, err)
		}
		if n == 0 {
			break
		}

		data := buffer[:n]
		blocks = append(blocks, NewBlock(seq, data))

		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			break
		}
	}

	if size == 0 && len(blocks) == 0 {
		return []Block{}, nil
	}

	return blocks, nil
}

func ReadBlock(reader io.ReadSeeker, block Block, blockSize int64) ([]byte, error) {
	offset := int64(block.Seq) * blockSize
	if _, err := reader.Seek(offset, io.SeekStart); err != nil {
		return nil, apperrors.Wrap(apperrors.KindFileSystem, err)
	}

	data := make([]byte, block.Size)
	if _, err := io.ReadFull(reader, data); err != nil {
		return nil, apperrors.Wrap(apperrors.KindFileSystem, err)
	}

	if got := Hash(data); got != block.Hash {
		return nil, apperrors.Errorf(apperrors.KindValidation, "block %d hash mismatch", block.Seq)
	}
	if got := Checksum(data); got != block.Checksum {
		return nil, apperrors.Errorf(apperrors.KindValidation, "block %d checksum mismatch", block.Seq)
	}

	return data, nil
}

func NewBlock(seq int, data []byte) Block {
	return Block{
		Hash:     Hash(data),
		Seq:      seq,
		Size:     int64(len(data)),
		Checksum: Checksum(data),
	}
}

func Hash(data []byte) string {
	hash := sha256.Sum256(data)
	return base64.StdEncoding.EncodeToString(hash[:])
}

func Checksum(data []byte) string {
	return strconv.FormatUint(uint64(adler32.Checksum(data)), 10)
}
