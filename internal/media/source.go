package media

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	apperrors "openminutes/internal/errors"
)

type SourceOptions struct {
	FilePath string
	Reader   io.ReadSeeker
	Size     int64
	Name     string
}

type Source struct {
	Reader io.ReadSeeker
	Size   int64
	Name   string
}

func OpenSource(options SourceOptions) (*Source, error) {
	name := strings.TrimSpace(options.Name)
	size := options.Size
	reader := options.Reader

	if reader == nil {
		if options.FilePath == "" {
			return nil, apperrors.New(apperrors.KindValidation, "upload file path or reader is required")
		}

		file, err := os.Open(options.FilePath)
		if err != nil {
			return nil, apperrors.Wrap(apperrors.KindFileSystem, err)
		}

		info, err := file.Stat()
		if err != nil {
			_ = file.Close()
			return nil, apperrors.Wrap(apperrors.KindFileSystem, err)
		}

		reader = file
		size = info.Size()
		if name == "" {
			name = filepath.Base(options.FilePath)
		}
	}

	if reader == nil {
		return nil, apperrors.New(apperrors.KindValidation, "upload reader is required")
	}
	if size < 0 {
		return nil, apperrors.New(apperrors.KindValidation, "upload size cannot be negative")
	}
	if size == 0 {
		current, err := reader.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, apperrors.New(apperrors.KindValidation, "upload size is required when reader cannot report size")
		}
		end, err := reader.Seek(0, io.SeekEnd)
		if err != nil {
			return nil, apperrors.New(apperrors.KindValidation, "upload size is required when reader cannot report size")
		}
		if _, err := reader.Seek(current, io.SeekStart); err != nil {
			return nil, apperrors.Wrap(apperrors.KindFileSystem, err)
		}
		size = end
	}
	if name == "" {
		name = "upload"
	}

	return &Source{
		Reader: reader,
		Size:   size,
		Name:   name,
	}, nil
}

func SeekStart(reader io.Seeker) error {
	_, err := reader.Seek(0, io.SeekStart)
	return err
}
