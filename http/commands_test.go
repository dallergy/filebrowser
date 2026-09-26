package fbhttp

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/users"
)

// dialCommand opens the command socket as a user with perm on a server with
// the given exec setting.
func dialCommand(t *testing.T, perm users.Permissions, enableExec bool) *websocket.Conn {
	t.Helper()

	key := []byte("test-signing-key")
	st := scopedUserStorage(t, t.TempDir(), perm, key)
	srv := httptest.NewServer(handle(commandsHandler, "/api/command", st, &settings.Server{EnableExec: enableExec}))
	t.Cleanup(srv.Close)

	header := http.Header{}
	header.Set("X-Auth", signToken(t, perm, key))
	conn, res, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/api/command/", header)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = res.Body.Close()
	t.Cleanup(func() { _ = conn.Close() })
	// A server that waits for input it should not need would otherwise hang the
	// test instead of failing it.
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	return conn
}

// Regression for GHSA-39cx-23x9-5c8p: the command socket read a whole message
// of any size before checking EnableExec and Perm.Execute, so any user could
// make the server buffer arbitrarily large payloads.
func TestCommandSocketRejectsBeforeReading(t *testing.T) {
	t.Run("user without exec is refused without sending anything", func(t *testing.T) {
		conn := dialCommand(t, users.Permissions{}, true)
		_, msg, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if !bytes.Equal(msg, cmdNotAllowed) {
			t.Errorf("got %q; want %q", msg, cmdNotAllowed)
		}
	})

	t.Run("oversized message is not buffered", func(t *testing.T) {
		conn := dialCommand(t, users.Permissions{Execute: true}, true)
		if err := conn.WriteMessage(websocket.TextMessage, bytes.Repeat([]byte("A"), maxCommandMessageSize+1)); err != nil {
			t.Fatalf("write: %v", err)
		}
		_, msg, err := conn.ReadMessage()
		var closeErr *websocket.CloseError
		if err == nil || !errors.As(err, &closeErr) {
			t.Fatalf("VULNERABLE: oversized message was read and answered with %q (err=%v)", msg, err)
		}
	})

	t.Run("regular sized command is still read", func(t *testing.T) {
		// The user may execute, but "ls" is not in their command list.
		conn := dialCommand(t, users.Permissions{Execute: true}, true)
		if err := conn.WriteMessage(websocket.TextMessage, []byte("ls")); err != nil {
			t.Fatalf("write: %v", err)
		}
		_, msg, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if !bytes.Equal(msg, cmdNotAllowed) {
			t.Errorf("got %q; want %q", msg, cmdNotAllowed)
		}
	})
}
