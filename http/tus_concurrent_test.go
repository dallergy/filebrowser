package fbhttp

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

// Regression for GHSA-4r8p-gqj2-mwgm: concurrent PATCHes at the same offset all
// passed the offset check before any of them wrote, and O_APPEND then stacked
// their chunks, so the file grew to a multiple of its declared Upload-Length
// and the completion hook fired more than once.
func TestTusConcurrentPatchesStayWithinUploadLength(t *testing.T) {
	const fileSize = 256 << 10
	const racers = 8

	f := newTusTestFixture(t)
	payload := testPayload(fileSize)
	uploadURL := f.create(t, "race.bin", fileSize)

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		accepted int
		bodies   []*io.PipeWriter
	)
	for range racers {
		// Hold every body back until all requests are in flight, so they all
		// reach the offset check before any chunk lands on disk.
		pr, pw := io.Pipe()
		bodies = append(bodies, pw)

		req, err := http.NewRequest(http.MethodPatch, uploadURL, pr)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-Auth", f.token)
		req.Header.Set("Content-Type", "application/offset+octet-stream")
		req.Header.Set("Upload-Offset", strconv.Itoa(0))

		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := f.client.Do(req)
			if err != nil {
				t.Errorf("PATCH: %v", err)
				return
			}
			_, _ = io.Copy(io.Discard, res.Body)
			res.Body.Close()
			if res.StatusCode == http.StatusNoContent {
				mu.Lock()
				accepted++
				mu.Unlock()
			}
		}()
	}

	time.Sleep(200 * time.Millisecond)
	for _, pw := range bodies {
		go func() {
			_, _ = pw.Write(payload)
			_ = pw.Close()
		}()
	}
	wg.Wait()

	info, err := os.Stat(filepath.Join(f.scope, "race.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != fileSize {
		t.Errorf("VULNERABLE: file is %d bytes; declared Upload-Length is %d", info.Size(), fileSize)
	}
	if accepted != 1 {
		t.Errorf("%d PATCHes completed the upload; want exactly 1", accepted)
	}
}
