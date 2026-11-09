// Package requestaudit stores bounded, encrypted request evidence independently
// of application logs and PostgreSQL. It never changes the forwarded request.
package requestaudit

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const segmentBytes int64 = 64 << 20

type Record struct {
	ClientRequestID   string          `json:"client_request_id,omitempty"`
	AttemptID         string          `json:"attempt_id,omitempty"`
	UpstreamRequestID string          `json:"upstream_request_id,omitempty"`
	HTTPStatus        int             `json:"http_status,omitempty"`
	TransportError    bool            `json:"transport_error,omitempty"`
	Time              time.Time       `json:"time"`
	RequestID         string          `json:"request_id"`
	UserID            int64           `json:"user_id"`
	APIKeyID          int64           `json:"api_key_id"`
	AccountID         int64           `json:"account_id"`
	ParentAccountID   *int64          `json:"parent_account_id,omitempty"`
	ProxyID           *int64          `json:"configured_proxy_id,omitempty"`
	Phase             string          `json:"phase"`
	Endpoint          string          `json:"endpoint,omitempty"`
	Model             string          `json:"model,omitempty"`
	BodyBytes         int             `json:"body_bytes"`
	BodySHA256        string          `json:"body_sha256"`
	Body              json.RawMessage `json:"body,omitempty"`
	Omitted           string          `json:"omitted,omitempty"`
	Redactions        int             `json:"redactions"`
}

// Each line is independently authenticated and gzip compressed before encryption.
type Envelope struct {
	Version int    `json:"version"`
	Nonce   []byte `json:"nonce"`
	Data    []byte `json:"data"`
}

type Store struct {
	Dir       string
	MaxBytes  int64
	Retention time.Duration
	aead      cipher.AEAD
	mu        sync.Mutex
}

func New(dir string, key []byte, maxBytes int64, retention time.Duration) (*Store, error) {
	if !filepath.IsAbs(dir) || len(key) != 32 || maxBytes < 1<<20 || retention <= 0 {
		return nil, errors.New("invalid request audit configuration")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Store{Dir: dir, MaxBytes: maxBytes, Retention: retention, aead: aead}, nil
}

var bearer = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]{12,}`)
var apiSecret = regexp.MustCompile(`\b(?:sk-(?:proj-|ant-)?|sess-)[A-Za-z0-9_-]{16,}`)
var jwt = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`)
var assignedSecret = regexp.MustCompile(`(?i)\b(?:api[_-]?key|access[_-]?token|refresh[_-]?token|client[_-]?secret|password|passwd|authorization)\s*[:=]\s*["']?[^\s,"';]{6,}`)

func redact(v any, count *int) any {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			n := strings.ToLower(strings.NewReplacer("-", "", "_", "").Replace(k))
			switch n {
			case "authorization", "proxyauthorization", "cookie", "setcookie", "apikey", "accesstoken", "refreshtoken", "idtoken", "clientsecret", "password", "passwd", "privatekey":
				x[k] = "[REDACTED]"
				*count++
			default:
				x[k] = redact(val, count)
			}
		}
	case []any:
		for i := range x {
			x[i] = redact(x[i], count)
		}
	case string:
		lower := strings.ToLower(x)
		checks := []struct {
			pattern  *regexp.Regexp
			possible bool
		}{
			{bearer, strings.Contains(lower, "bearer")},
			{apiSecret, strings.Contains(x, "sk-") || strings.Contains(x, "sess-")},
			{jwt, strings.Contains(x, "eyJ")},
			{assignedSecret, strings.Contains(lower, "key") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "passwd") || strings.Contains(lower, "authorization")},
		}
		for _, check := range checks {
			if check.possible {
				x = check.pattern.ReplaceAllStringFunc(x, func(string) string { *count++; return "[REDACTED]" })
			}
		}
		return x
	}
	return v
}

// Write records every invocation synchronously, without a lossy in-memory queue.
// Errors must be counted and reported by the caller; forwarding remains fail-open.
func (s *Store) Write(record Record, body []byte) error {
	record.Time = time.Now().UTC()
	record.BodyBytes = len(body)
	sum := sha256.Sum256(body)
	record.BodySHA256 = hex.EncodeToString(sum[:])
	var parsed any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if body == nil {
		record.Omitted = "no_body"
	} else if !json.Valid(body) {
		record.Omitted = "non_json_body"
	} else if err := decoder.Decode(&parsed); err != nil {
		return err
	} else {
		clean := redact(parsed, &record.Redactions)
		var err error
		record.Body, err = json.Marshal(clean)
		if err != nil {
			return err
		}
	}
	plain, err := json.Marshal(record)
	if err != nil {
		return err
	}
	var compressed bytes.Buffer
	z, err := gzip.NewWriterLevel(&compressed, gzip.BestSpeed)
	if err != nil {
		return err
	}
	if _, err = z.Write(plain); err != nil {
		return err
	}
	if err = z.Close(); err != nil {
		return err
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return err
	}
	line, err := json.Marshal(Envelope{1, nonce, s.aead.Seal(nil, nonce, compressed.Bytes(), []byte("sub2api-request-audit-v1"))})
	if err != nil {
		return err
	}
	line = append(line, '\n')
	if int64(len(line)) > s.MaxBytes {
		return errors.New("request audit record exceeds total storage quota")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := os.OpenFile(filepath.Join(s.Dir, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	now := time.Now()
	// Segment names encode creation time, independent of the last append.
	// Expire whole segments from their start: records may expire up to an hour
	// early, but continuous traffic can never prolong retention.
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return err
	}
	current := ""
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		started, ok := segmentTime(e.Name())
		if !ok || e.IsDir() {
			continue
		}
		st, err := e.Info()
		if err != nil {
			return err
		}
		if now.Sub(started) >= 0 && now.Sub(started) < time.Hour && st.Size()+int64(len(line)) <= segmentBytes {
			current = filepath.Join(s.Dir, e.Name())
		}
		break
	}
	if current == "" {
		current = filepath.Join(s.Dir, fmt.Sprintf("audit-%020d.jsonl", now.UnixNano()))
	}
	if err = s.pruneLocked(now, int64(len(line))); err != nil {
		return err
	}
	f, err := os.OpenFile(current, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	_, writeErr := f.Write(line)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

// Prune also runs when no requests arrive, so expiry does not depend on traffic.
func (s *Store) Prune() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(filepath.Join(s.Dir, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return s.pruneLocked(time.Now(), 0)
}

func (s *Store) pruneLocked(now time.Time, incoming int64) error {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return err
	}
	type file struct {
		name string
		size int64
		at   time.Time
	}
	var files []file
	var total int64
	for _, e := range entries {
		started, ok := segmentTime(e.Name())
		if e.IsDir() || !ok {
			continue
		}
		st, err := e.Info()
		if err != nil {
			return err
		}
		p := filepath.Join(s.Dir, e.Name())
		if now.Sub(started) >= s.Retention {
			if err = os.Remove(p); err != nil {
				return err
			}
			continue
		}
		files = append(files, file{p, st.Size(), started})
		total += st.Size()
	}
	sort.Slice(files, func(i, j int) bool { return files[i].at.Before(files[j].at) })
	for _, f := range files {
		if total+incoming <= s.MaxBytes {
			break
		}
		if err = os.Remove(f.name); err != nil {
			return err
		}
		total -= f.size
	}
	return nil
}

func (s *Store) Decode(line []byte) (Record, error) {
	var env Envelope
	var r Record
	if err := json.Unmarshal(line, &env); err != nil {
		return r, err
	}
	if env.Version != 1 || len(env.Nonce) != s.aead.NonceSize() {
		return r, errors.New("invalid audit envelope")
	}
	b, err := s.aead.Open(nil, env.Nonce, env.Data, []byte("sub2api-request-audit-v1"))
	if err != nil {
		return r, err
	}
	z, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return r, err
	}
	defer z.Close()
	b, err = io.ReadAll(z)
	if err != nil {
		return r, err
	}
	err = json.Unmarshal(b, &r)
	return r, err
}

func segmentTime(name string) (time.Time, bool) {
	if !strings.HasPrefix(name, "audit-") || !strings.HasSuffix(name, ".jsonl") {
		return time.Time{}, false
	}
	n, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(name, "audit-"), ".jsonl"), 10, 64)
	return time.Unix(0, n), err == nil && n > 0
}
