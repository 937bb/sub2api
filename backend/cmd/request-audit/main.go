// request-audit reads local encrypted audit files. It never contacts an API.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/requestaudit"
)

func main() {
	dir := flag.String("dir", "", "audit directory")
	keyFile := flag.String("key-file", "", "32-byte AES key file")
	account := flag.Int64("account", 0, "upstream account filter")
	user := flag.Int64("user", 0, "user filter")
	request := flag.String("request", "", "request ID filter")
	since := flag.String("since", "", "RFC3339 start time")
	body := flag.Bool("body", false, "include decrypted bodies (sensitive)")
	limit := flag.Int("limit", 100, "maximum matching records; 0 = all")
	flag.Parse()
	fail := func(e error) { fmt.Fprintln(os.Stderr, e); os.Exit(1) }
	key, e := os.ReadFile(*keyFile)
	if e != nil {
		fail(e)
	}
	s, e := requestaudit.New(*dir, key, 5<<30, 72*time.Hour)
	if e != nil {
		fail(e)
	}
	var start time.Time
	if *since != "" {
		start, e = time.Parse(time.RFC3339, *since)
		if e != nil {
			fail(e)
		}
	}
	paths, e := filepath.Glob(filepath.Join(*dir, "audit-*.jsonl"))
	if e != nil {
		fail(e)
	}
	enc := json.NewEncoder(os.Stdout)
	count := 0
	for _, p := range paths {
		f, e := os.Open(p)
		if e != nil {
			if os.IsNotExist(e) {
				continue
			}
			fail(e)
		}
		reader := bufio.NewReader(f)
		for {
			line, err := reader.ReadBytes('\n')
			if len(line) == 0 {
				break
			}
			if line[len(line)-1] != '\n' {
				break
			} // active final append may be incomplete
			r, e := s.Decode(line)
			if e != nil {
				fail(fmt.Errorf("decode %s: %w", filepath.Base(p), e))
			}
			if (*account == 0 || r.AccountID == *account) && (*user == 0 || r.UserID == *user) && (*request == "" || r.RequestID == *request) && !r.Time.Before(start) {
				if !*body {
					r.Body = nil
				}
				if e := enc.Encode(r); e != nil {
					fail(e)
				}
				count++
				if *limit > 0 && count >= *limit {
					f.Close()
					return
				}
			}
			if err != nil {
				break
			}
		}
		f.Close()
	}
}
