package requestaudit

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(t.TempDir(), bytes.Repeat([]byte{7}, 32), 1<<20, 72*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func records(t *testing.T, s *Store) []Record {
	t.Helper()
	paths, _ := filepath.Glob(filepath.Join(s.Dir, "audit-*.jsonl"))
	var out []Record
	for _, p := range paths {
		b, e := os.ReadFile(p)
		if e != nil {
			t.Fatal(e)
		}
		for _, line := range bytes.Split(bytes.TrimSpace(b), []byte{'\n'}) {
			if len(line) == 0 {
				continue
			}
			r, e := s.Decode(line)
			if e != nil {
				t.Fatal(e)
			}
			out = append(out, r)
		}
	}
	return out
}
func TestEncryptedRedactedAndUnchanged(t *testing.T) {
	s := testStore(t)
	body := []byte(`{"input":[{"role":"user","content":"unique-private-prompt Bearer abcdefghijklmnop"}],"access_token":"secret","number":9007199254740993}`)
	original := bytes.Clone(body)
	if e := s.Write(Record{AccountID: 42, UserID: 3, APIKeyID: 7, RequestID: "req-test"}, body); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(body, original) {
		t.Fatal("forwarding body modified")
	}
	paths, _ := filepath.Glob(filepath.Join(s.Dir, "audit-*"))
	raw, _ := os.ReadFile(paths[0])
	if bytes.Contains(raw, []byte("unique-private-prompt")) || bytes.Contains(raw, []byte("secret")) {
		t.Fatal("plaintext leaked")
	}
	r := records(t, s)[0]
	if r.AccountID != 42 || r.UserID != 3 || r.APIKeyID != 7 || r.RequestID != "req-test" || r.Redactions != 2 {
		t.Fatalf("metadata: %+v", r)
	}
	if !bytes.Contains(r.Body, []byte("unique-private-prompt")) || !bytes.Contains(r.Body, []byte("9007199254740993")) || bytes.Contains(r.Body, []byte("abcdefghijklmnop")) {
		t.Fatal(string(r.Body))
	}
	var env Envelope
	json.Unmarshal(bytes.TrimSpace(raw), &env)
	env.Data[0] ^= 1
	bad, _ := json.Marshal(env)
	if _, e := s.Decode(bad); e == nil {
		t.Fatal("tampering accepted")
	}
	st, _ := os.Stat(paths[0])
	if st.Mode().Perm() != 0600 {
		t.Fatal(st.Mode())
	}
}
func TestRetentionUsesCreationNotLatestWrite(t *testing.T) {
	s := testStore(t)
	old := filepath.Join(s.Dir, fmt.Sprintf("audit-%020d.jsonl", time.Now().Add(-73*time.Hour).UnixNano()))
	if e := os.WriteFile(old, []byte("recently touched"), 0600); e != nil {
		t.Fatal(e)
	}
	keep := filepath.Join(s.Dir, "operator-note.txt")
	os.WriteFile(keep, []byte("keep"), 0600)
	if e := s.Prune(); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(old); !os.IsNotExist(e) {
		t.Fatal("expired segment survived")
	}
	if _, e := os.Stat(keep); e != nil {
		t.Fatal(e)
	}
	// A segment older than one hour must not be reopened even with a fresh mtime.
	name := filepath.Join(s.Dir, fmt.Sprintf("audit-%020d.jsonl", time.Now().Add(-2*time.Hour).UnixNano()))
	os.WriteFile(name, nil, 0600)
	if e := s.Write(Record{}, []byte(`{"x":1}`)); e != nil {
		t.Fatal(e)
	}
	st, _ := os.Stat(name)
	if st.Size() != 0 {
		t.Fatal("appended to old segment")
	}
}
func TestSharedQuotaAndConcurrentWriters(t *testing.T) {
	s := testStore(t)
	second, e := New(s.Dir, bytes.Repeat([]byte{7}, 32), s.MaxBytes, s.Retention)
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v := s
			if i%2 == 0 {
				v = second
			}
			if e := v.Write(Record{AccountID: int64(i)}, []byte(`{"text":"parallel"}`)); e != nil {
				t.Error(e)
			}
		}(i)
	}
	wg.Wait()
	if len(records(t, s)) != 30 {
		t.Fatal("lost writes")
	}
	b := make([]byte, 450000)
	rand.Read(b)
	body, _ := json.Marshal(map[string]string{"data": base64.StdEncoding.EncodeToString(b)})
	for i := 0; i < 4; i++ {
		if e := second.Write(Record{}, body); e != nil {
			t.Fatal(e)
		}
	}
	var size int64
	paths, _ := filepath.Glob(filepath.Join(s.Dir, "audit-*.jsonl"))
	for _, p := range paths {
		st, _ := os.Stat(p)
		size += st.Size()
	}
	if size > s.MaxBytes {
		t.Fatalf("quota %d > %d", size, s.MaxBytes)
	}
	if len(records(t, s)) != 1 {
		t.Fatal("oldest records not removed")
	}
}
func TestNonJSONIsExplicitlyOmitted(t *testing.T) {
	s := testStore(t)
	if e := s.Write(Record{}, []byte("password=secret")); e != nil {
		t.Fatal(e)
	}
	r := records(t, s)[0]
	if r.Omitted != "non_json_body" || len(r.Body) != 0 || r.BodyBytes == 0 {
		t.Fatal(r)
	}
}
func BenchmarkWrite200KB(b *testing.B) {
	s, _ := New(b.TempDir(), bytes.Repeat([]byte{1}, 32), 5<<30, 72*time.Hour)
	body, _ := json.Marshal(map[string]string{"input": string(bytes.Repeat([]byte("normal request "), 15000))})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if e := s.Write(Record{}, body); e != nil {
			b.Fatal(e)
		}
	}
}
