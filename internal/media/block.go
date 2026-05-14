package media

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"hash/adler32"
	"io"
	"strconv"
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
		return nil, errors.New("prepare response missing block_size")
	}
	if err := SeekStart(reader); err != nil {
		return nil, err
	}

	var blocks []Block
	buffer := make([]byte, blockSize)
	for seq := 0; ; seq++ {
		n, err := io.ReadFull(reader, buffer)
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
			return nil, err
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
		return nil, err
	}

	data := make([]byte, block.Size)
	if _, err := io.ReadFull(reader, data); err != nil {
		return nil, err
	}

	if got := Hash(data); got != block.Hash {
		return nil, fmt.Errorf("block %d hash mismatch", block.Seq)
	}
	if got := Checksum(data); got != block.Checksum {
		return nil, fmt.Errorf("block %d checksum mismatch", block.Seq)
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
