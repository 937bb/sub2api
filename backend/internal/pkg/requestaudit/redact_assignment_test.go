package requestaudit

import (
	"bytes"
	"strings"
	"testing"
)

func checkAssignmentEquivalent(t *testing.T, input string) {
	t.Helper()
	wantCount := 0
	want := assignedSecret.ReplaceAllStringFunc(input, func(string) string { wantCount++; return "[REDACTED]" })
	count := 0
	got := redactAssignments(input, &count)
	if got != want || count != wantCount {
		t.Fatalf("input=%q got=%q count=%d want=%q count=%d", input, got, count, want, wantCount)
	}
}

func TestAssignmentRedactionEquivalent(t *testing.T) {
	for _, key := range assignedSecretPrefixes {
		for _, prefix := range []string{"", "word", "_", "7", "-", "中文", "İ", "İK", "\n"} {
			for _, sep := range []string{"=", ":", " \t\r\n\f: ", "\v=", "\u00a0="} {
				for _, value := range []string{"short", "secret-value", "\"secret-value\"", "'secret-value'", "abcdef;other", "abcdef,password=another-secret", strings.Repeat("x", 10000)} {
					checkAssignmentEquivalent(t, prefix+strings.ToUpper(key)+sep+value+" password=tailsecret")
				}
			}
		}
	}
	for _, input := range []string{"paſſword=abcdefgh", "apiKey=abcdefgh", "ACCESS_TOKEN=secret-value", "password=authorization:abcdefgh", "password" + strings.Repeat(" ", 100000) + "=abcdef", strings.Repeat("password ", 10000), "password=secret\xffvalue"} {
		checkAssignmentEquivalent(t, input)
	}
}

func FuzzAssignmentRedactionEquivalent(f *testing.F) {
	for _, seed := range []string{"password=secret-value", "api_key: 'abcdef'", "中文 paſſword = secret123", "access_token=abcdef, password=ghijkl", "key token client normal code"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) { checkAssignmentEquivalent(t, input) })
}

func TestWriteJSONValidationAndNumbers(t *testing.T) {
	for _, input := range []string{`{"x":1} {"y":2}`, `{"x":1}garbage`, `{"x":`, ``, `null null`} {
		t.Run(input, func(t *testing.T) {
			s := testStore(t)
			if err := s.Write(Record{}, []byte(input)); err != nil {
				t.Fatal(err)
			}
			record := records(t, s)[0]
			if record.Omitted != "non_json_body" || len(record.Body) != 0 {
				t.Fatalf("invalid body retained: %+v", record)
			}
		})
	}
	for _, input := range []string{`null`, `123456789012345678901234567890`, `{"n":9007199254740993}`, "[1,2]\n \t"} {
		t.Run(input, func(t *testing.T) {
			s := testStore(t)
			if err := s.Write(Record{}, []byte(input)); err != nil {
				t.Fatal(err)
			}
			record := records(t, s)[0]
			if record.Omitted != "" || !bytes.Equal(record.Body, bytes.TrimSpace([]byte(input))) {
				t.Fatalf("valid body changed: %+v", record)
			}
		})
	}
}

func TestCompressionReuseKeepsRecordsIndependent(t *testing.T) {
	s := testStore(t)
	inputs := []string{`{"message":"first-private-text","api_key":"first-secret"}`, `{"message":"second-private-text","password":"second-secret"}`, `null`, `{"message":"third-private-text"}`}
	for i, input := range inputs {
		if err := s.Write(Record{AccountID: int64(i + 1)}, []byte(input)); err != nil {
			t.Fatal(err)
		}
	}
	result := records(t, s)
	if len(result) != len(inputs) {
		t.Fatal("lost records")
	}
	for i, record := range result {
		if record.AccountID != int64(i+1) {
			t.Fatal("wrong record identity")
		}
		for j, marker := range []string{"first-private-text", "second-private-text", "", "third-private-text"} {
			if marker != "" && bytes.Contains(record.Body, []byte(marker)) != (i == j) {
				t.Fatal("cross-record compression contents")
			}
		}
		if bytes.Contains(record.Body, []byte("first-secret")) || bytes.Contains(record.Body, []byte("second-secret")) {
			t.Fatal("credential retained")
		}
	}
}
