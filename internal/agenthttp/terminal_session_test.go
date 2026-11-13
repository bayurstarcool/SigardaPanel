package agenthttp

import (
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func dialTerminal(t *testing.T, srv *httptest.Server, query string) *websocket.Conn {
	t.Helper()
	u := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/terminal?" + query
	ws, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return ws
}

func TestTerminalExitEndsSession(t *testing.T) {
	srv := httptest.NewServer(NewRouterWithOptions(RouterOptions{}))
	defer srv.Close()
	ws := dialTerminal(t, srv, "session=u1-exit-test")
	defer ws.Close()
	ws.WriteMessage(websocket.TextMessage, []byte("exit\n"))
	ws.SetReadDeadline(time.Now().Add(8 * time.Second))
	for {
		if _, _, err := ws.ReadMessage(); err != nil {
			if strings.Contains(err.Error(), "timeout") {
				t.Fatal("the websocket stayed open after the shell exited")
			}
			break
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := getOrCreateSession("u1-exit-test"); !ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the session was not removed after the shell exited")
}

func TestTerminalReattachWhileStreaming(t *testing.T) {
	srv := httptest.NewServer(NewRouterWithOptions(RouterOptions{}))
	defer srv.Close()
	const id = "u1-race-test"
	first := dialTerminal(t, srv, "session="+id)
	first.WriteMessage(websocket.TextMessage, []byte("for i in $(seq 1 3000); do echo line$i; done\n"))
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		ws := dialTerminal(t, srv, "session="+id)
		wg.Add(1)
		go func(c *websocket.Conn) {
			defer wg.Done()
			c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			for {
				if _, _, err := c.ReadMessage(); err != nil {
					return
				}
			}
		}(ws)
		time.Sleep(30 * time.Millisecond)
	}
	wg.Wait()
	first.Close()
	removeSession(id)
}
