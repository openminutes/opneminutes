package media

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	MaxUploadFileSize = 6 * 1024 * 1024 * 1024
	MaxUploadDuration = 6 * time.Hour
)

var SupportedUploadExtensions = map[string]struct{}{
	".wav":  {},
	".mp3":  {},
	".m4a":  {},
	".aac":  {},
	".ogg":  {},
	".wma":  {},
	".amr":  {},
	".avi":  {},
	".wmv":  {},
	".mov":  {},
	".mp4":  {},
	".m4v":  {},
	".mpeg": {},
	".flv":  {},
}

func ValidateUploadFile(filePath string, logger *zap.Logger) error {
	if logger == nil {
		logger = zap.NewNop()
	}

	info, err := os.Stat(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("file %q does not exist", filePath)
		}
		return fmt.Errorf("stat file %q: %w", filePath, err)
	}
	if info.IsDir() {
		return fmt.Errorf("file %q is a directory", filePath)
	}

	extension := strings.ToLower(filepath.Ext(filePath))
	if _, ok := SupportedUploadExtensions[extension]; !ok {
		return fmt.Errorf("unsupported file extension %q", extension)
	}

	if info.Size() > MaxUploadFileSize {
		return fmt.Errorf("file size %d exceeds maximum %d bytes", info.Size(), int64(MaxUploadFileSize))
	}

	duration, known, err := ProbeUploadDuration(filePath, extension)
	if err != nil {
		logger.Debug("upload duration probe failed",
			zap.String("path", filePath),
			zap.String("extension", extension),
			zap.Error(err),
		)
		return nil
	}
	if !known {
		logger.Debug("upload duration probe skipped",
			zap.String("path", filePath),
			zap.String("extension", extension),
		)
		return nil
	}
	if duration > MaxUploadDuration {
		return fmt.Errorf("file duration %s exceeds maximum %s", duration, MaxUploadDuration)
	}

	logger.Debug("upload duration probe completed",
		zap.String("path", filePath),
		zap.String("extension", extension),
		zap.Duration("duration", duration),
	)
	return nil
}
