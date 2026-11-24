package smartws

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestMultilineErrorEmitsCompleteJSONDataLine(t *testing.T) {
	payload := `{
  "type": "error",
  "error": {
    "type": "invalid_request_error",
    "code": "invalid_value",
    "param": "input[0].content[1].file_data",
    "message": "unsupported MIME type image/mpo"
  }
}`
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		if _, _, err = ws.ReadMessage(); err != nil {
			return
		}
		_ = ws.WriteMessage(websocket.TextMessage, []byte(payload))
		_, _, _ = ws.ReadMessage()
	}))
	t.Cleanup(s.Close)
	p := testPool(t, 8)
	out := consume(t, p, request(t, s.URL, validBody, "multiline"), "", "scope")
	count := 0
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		count++
		data := strings.TrimPrefix(line, "data: ")
		if !json.Valid([]byte(data)) {
			t.Fatalf("incomplete JSON data line: %q", data)
		}
		if !strings.Contains(data, "image/mpo") {
			t.Fatal("upstream error detail lost")
		}
	}
	if count != 1 {
		t.Fatalf("expected one JSON data line, got %d", count)
	}
}
