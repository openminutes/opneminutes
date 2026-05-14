package media

import (
	"errors"
	"io"
	"os"
	"time"

	apperrors "openminutes/internal/errors"

	mp4 "github.com/Eyevinn/mp4ff/mp4"
	"github.com/go-audio/wav"
	"github.com/jfreymuth/oggvorbis"
	"github.com/tcolgate/mp3"
)

const maxTimeDuration = time.Duration(1<<63 - 1)

var ErrUploadDurationUnknown = errors.New("upload duration is unknown")

func ProbeUploadDuration(filePath, extension string) (time.Duration, bool, error) {
	switch extension {
	case ".mp4", ".m4v", ".m4a", ".mov":
		return ProbeUploadDurationFile(filePath, ProbeMP4Duration)
	case ".wav":
		return ProbeUploadDurationFile(filePath, ProbeWAVDuration)
	case ".mp3":
		return ProbeUploadDurationFile(filePath, ProbeMP3Duration)
	case ".ogg":
		return ProbeUploadDurationFile(filePath, ProbeOggVorbisDuration)
	default:
		return 0, false, nil
	}
}

func ProbeUploadDurationFile(filePath string, probe func(*os.File) (time.Duration, error)) (time.Duration, bool, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, false, apperrors.Wrap(apperrors.KindFileSystem, err)
	}
	defer file.Close()

	duration, err := probe(file)
	if err != nil {
		return 0, false, err
	}
	if duration <= 0 {
		return 0, false, ErrUploadDurationUnknown
	}

	return duration, true, nil
}

func ProbeMP4Duration(file *os.File) (time.Duration, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}

	parsed, err := mp4.DecodeFile(file, mp4.WithDecodeMode(mp4.DecModeLazyMdat))
	if err != nil {
		return 0, err
	}

	if parsed.Moov == nil || parsed.Moov.Mvhd == nil || parsed.Moov.Mvhd.Timescale == 0 || parsed.Moov.Mvhd.Duration == 0 {
		return 0, ErrUploadDurationUnknown
	}

	return DurationFromTimeUnits(parsed.Moov.Mvhd.Duration, uint64(parsed.Moov.Mvhd.Timescale)), nil
}

func ProbeWAVDuration(file *os.File) (time.Duration, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}

	decoder := wav.NewDecoder(file)
	if err := decoder.FwdToPCM(); err != nil {
		return 0, err
	}
	if decoder.NumChans < 1 || decoder.BitDepth < 8 || decoder.AvgBytesPerSec == 0 || decoder.PCMSize <= 0 {
		return 0, ErrUploadDurationUnknown
	}

	return DurationFromTimeUnits(uint64(decoder.PCMSize), uint64(decoder.AvgBytesPerSec)), nil
}

func ProbeMP3Duration(file *os.File) (time.Duration, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}

	decoder := mp3.NewDecoder(file)
	var frame mp3.Frame
	var duration time.Duration
	for {
		var skipped int
		if err := decoder.Decode(&frame, &skipped); err != nil {
			if errors.Is(err, io.EOF) && duration > 0 {
				return duration, nil
			}
			return 0, err
		}

		duration += frame.Duration()
		if duration > MaxUploadDuration {
			return duration, nil
		}
	}
}

func ProbeOggVorbisDuration(file *os.File) (time.Duration, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}

	samples, format, err := oggvorbis.GetLength(file)
	if err != nil {
		return 0, err
	}
	if format == nil || format.SampleRate <= 0 || samples <= 0 {
		return 0, ErrUploadDurationUnknown
	}

	return DurationFromTimeUnits(uint64(samples), uint64(format.SampleRate)), nil
}

func DurationFromTimeUnits(units, unitsPerSecond uint64) time.Duration {
	if units == 0 || unitsPerSecond == 0 {
		return 0
	}

	wholeSeconds := units / unitsPerSecond
	maxWholeSeconds := uint64(maxTimeDuration / time.Second)
	if wholeSeconds > maxWholeSeconds {
		return maxTimeDuration
	}

	remainder := units % unitsPerSecond
	duration := time.Duration(wholeSeconds) * time.Second
	nanos := time.Duration((remainder * uint64(time.Second)) / unitsPerSecond)
	if maxTimeDuration-duration < nanos {
		return maxTimeDuration
	}

	return duration + nanos
}
