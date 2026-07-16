package service

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func TestSplitOpenAIConcatenatedJSONDocuments(t *testing.T) {
	valid := `{"type":"response.in_progress","text":"a}\\\"b"} {"type":"response.completed","response":{"usage":{"input_tokens":1}}}`
	docs, outcome := splitOpenAIConcatenatedJSONDocuments([]byte(valid))
	if outcome != openAIJSONDocumentsRepaired || len(docs) != 2 || !bytes.Contains(docs[1], []byte(`"input_tokens":1`)) {
		t.Fatalf("split = %q, %v", docs, outcome)
	}

	unchanged := []string{
		`{"type":"response.completed"}`,
		strings.Repeat(" ", maxOpenAIConcatenatedJSONBytes+1),
	}
	for _, input := range unchanged {
		if got, gotOutcome := splitOpenAIConcatenatedJSONDocuments([]byte(input)); gotOutcome != openAIJSONDocumentsUnchanged {
			t.Errorf("outcome for ordinary input = %v, documents %q", gotOutcome, got)
		}
	}

	rejected := []string{
		`{"type":"a"}{bad}`,
		`{"type":"a"}junk`,
		`{"x":1}{"type":"b"}`,
		`{"type":"a\nb"}{"type":"b"}`,
		strings.Repeat(`{"type":"x"}`, maxOpenAIConcatenatedJSONDocuments+1),
		`{"type":"a","padding":"` + strings.Repeat("x", maxOpenAIConcatenatedJSONBytes) + `"}{"type":"b"}`,
	}
	for _, input := range rejected {
		if got, gotOutcome := splitOpenAIConcatenatedJSONDocuments([]byte(input)); gotOutcome != openAIJSONDocumentsRejected {
			t.Errorf("outcome for %q = %v, documents %q", input[:min(len(input), 80)], gotOutcome, got)
		}
	}
}

func TestSplitOpenAIConcatenatedJSONDocumentsOrdinaryEventSkipsDecoder(t *testing.T) {
	decodeCalls := 0
	openAIConcatenatedJSONDecodeHook = func() { decodeCalls++ }
	t.Cleanup(func() { openAIConcatenatedJSONDecodeHook = nil })

	payload := []byte(`{"type":"response.output_text.delta","delta":"ordinary"}`)
	documents, outcome := splitOpenAIConcatenatedJSONDocuments(payload)
	if outcome != openAIJSONDocumentsUnchanged || documents != nil || decodeCalls != 0 {
		t.Fatalf("documents=%q outcome=%v decodeCalls=%d", documents, outcome, decodeCalls)
	}
}

func TestOpenAISSEJSONDocumentScannerFramesDocuments(t *testing.T) {
	scanner := bufio.NewScanner(strings.NewReader("event: response.in_progress\ndata: {\"type\":\"response.in_progress\"} {\"type\":\"response.completed\",\"text\":\"x y\"}\n\n"))
	documents := newOpenAISSEJSONDocumentScanner(scanner)
	var lines []string
	for documents.Scan() {
		lines = append(lines, documents.Text())
	}
	want := []string{"event: response.in_progress", "event: response.in_progress", `data: {"type":"response.in_progress"}`, "", "event: response.completed", `data: {"type":"response.completed","text":"x y"}`, "", ""}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Fatalf("lines = %#v, want %#v", lines, want)
	}
}
