package fbhttp

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/asticode/go-astisub"

	fberrors "github.com/filebrowser/filebrowser/v2/errors"
	"github.com/filebrowser/filebrowser/v2/files"
)

var srtLineBreakTag = regexp.MustCompile(`(?i)<br(?:\s+[^>]*)?\s*/?>`)

// maxSubtitleSize bounds the subtitle files converted to WebVTT. Conversion
// holds the whole file in memory, several times over while parsing, so it gets
// the same 10MB ceiling as text files opened in the editor. Real subtitles are
// orders of magnitude smaller.
const maxSubtitleSize = 10 << 20 // 10MB

var subtitleHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	if !d.user.Perm.Download {
		return http.StatusAccepted, nil
	}

	file, err := files.NewFileInfo(&files.FileOptions{
		Fs:         d.user.Fs,
		Path:       r.URL.Path,
		Modify:     d.user.Perm.Modify,
		Expand:     false,
		ReadHeader: d.server.TypeDetectionByHeader,
		Checker:    d,
	})
	if err != nil {
		return errToStatus(err), err
	}

	if file.IsDir {
		return http.StatusBadRequest, nil
	}

	return subtitleFileHandler(w, r, file)
})

func subtitleFileHandler(w http.ResponseWriter, r *http.Request, file *files.FileInfo) (int, error) {
	// if its not a subtitle file, reject
	if !files.IsSupportedSubtitle(file.Name) {
		return http.StatusBadRequest, nil
	}

	fd, err := file.Fs.Open(file.Path)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	defer fd.Close()

	// load subtitle for conversion to vtt
	var sub *astisub.Subtitles
	isSRT := strings.HasSuffix(file.Name, ".srt")
	if isSRT || strings.HasSuffix(file.Name, ".ass") || strings.HasSuffix(file.Name, ".ssa") {
		content, readErr := readSubtitle(fd, file.Size)
		if readErr != nil {
			return errToStatus(readErr), readErr
		}
		if isSRT {
			sub, err = astisub.ReadFromSRT(bytes.NewReader(normalizeSRTLineBreaks(content)))
		} else {
			sub, err = astisub.ReadFromSSA(bytes.NewReader(content))
		}
	}
	if err != nil {
		return http.StatusInternalServerError, err
	}

	setContentDisposition(w, r, file)
	w.Header().Add("Content-Security-Policy", `script-src 'none';`)
	w.Header().Set("Cache-Control", "private")
	// force type to text/vtt
	w.Header().Set("Content-Type", "text/vtt")

	// serve vtt file directly
	if sub == nil {
		http.ServeContent(w, r, file.Name, file.ModTime, fd)
		return 0, nil
	}

	// convert others to vtt and serve from buffer
	var buf = &bytes.Buffer{}
	err = sub.WriteToWebVTT(buf)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	http.ServeContent(w, r, file.Name, file.ModTime, bytes.NewReader(buf.Bytes()))
	return 0, nil
}

// readSubtitle reads a subtitle file for conversion, refusing one larger than
// maxSubtitleSize. The read itself is bounded too, since the file can grow
// after it was stat'ed.
func readSubtitle(r io.Reader, size int64) ([]byte, error) {
	errTooLarge := fmt.Errorf("subtitle file exceeds %d bytes: %w", maxSubtitleSize, fberrors.ErrInvalidRequestParams)
	if size > maxSubtitleSize {
		return nil, errTooLarge
	}

	content, err := io.ReadAll(io.LimitReader(r, maxSubtitleSize+1))
	if err != nil {
		return nil, err
	}
	if len(content) > maxSubtitleSize {
		return nil, errTooLarge
	}

	return content, nil
}

func normalizeSRTLineBreaks(content []byte) []byte {
	return srtLineBreakTag.ReplaceAll(content, []byte("\n"))
}
