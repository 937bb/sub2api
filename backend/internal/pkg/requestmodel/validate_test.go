package requestmodel

import (
	"bytes"
	"mime/multipart"
	"testing"
)

func TestValidateJSONSelectors(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		reject     bool
	}{
		{"single", `{"model":"gpt-6-astra","input":"hi"}`, false},
		{"messages", `{"model":"gpt-6-astra","messages":[{"role":"user","content":"hi"}]}`, false},
		{"duplicates", `{"model":"gpt-6-astra","model":"gpt-6.1-sol"}`, true},
		{"reverse", `{"model":"gpt-6.1-sol","model":"gpt-6-astra"}`, true},
		{"identical", `{"model":"x","model":"x"}`, true},
		{"null_first", `{"model":null,"model":"x"}`, true},
		{"null_last", `{"model":"x","model":null}`, true},
		{"escaped_key", `{"model":"x","\u006dodel":"y"}`, true},
		{"variant", `{"model":"x","Model":"y"}`, true},
		{"variant_only", `{"MODEL":"x"}`, true},
		{"bom", "\xef\xbb\xbf" + `{"model":"x","model":"y"}`, true},
		{"control", "{\"input\":\"hi\nthere\",\"model\":\"x\",\"model\":\"y\"}", true},
		{"session", `{"type":"session.update","session":{"model":"x"}}`, false},
		{"duplicate_session", `{"session":{"model":"x"},"session":{"model":"y"}}`, true},
		{"nested_model", `{"session":{"model":"x","model":"y"}}`, true},
		{"nested_variant", `{"session":{"Model":"x"}}`, true},
		{"duplicate_event_type", `{"type":"session.update","type":"response.create","model":"x"}`, true},
		{"tool_schema", `{"model":"x","tools":[{"parameters":{"properties":{"model":{"type":"string"},"session":{"type":"object"}}}}],"input":[{"content":"model","model":"user data"}]}`, false},
		{"unrelated_duplicates", `{"model":"x","metadata":{"model":"a","model":"b"}}`, false},
		{"omitted_followup", `{"type":"response.create","input":"hi"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateJSONSelectors([]byte(tc.body))
			if (err != nil) != tc.reject {
				t.Fatalf("reject=%v, error=%v", tc.reject, err)
			}
		})
	}
}

func TestValidateMultipartSelectors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		fields [][2]string
		reject bool
	}{
		{"single", [][2]string{{"model", "x"}}, false},
		{"duplicate", [][2]string{{"model", "x"}, {"model", "y"}}, true},
		{"case", [][2]string{{"model", "x"}, {"Model", "y"}}, true},
		{"session", [][2]string{{"session", `{"model":"x","model":"y"}`}}, true},
		{"duplicate_session", [][2]string{{"session", `{"model":"x"}`}, {"session", `{"model":"y"}`}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			file, err := writer.CreateFormFile("image", "test.png")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = file.Write(bytes.Repeat([]byte("image-data"), 10000)); err != nil {
				t.Fatal(err)
			}
			for _, field := range tc.fields {
				if err := writer.WriteField(field[0], field[1]); err != nil {
					t.Fatal(err)
				}
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			before := append([]byte(nil), body.Bytes()...)
			err = ValidateBodySelectors(writer.FormDataContentType(), body.Bytes())
			if (err != nil) != tc.reject {
				t.Fatalf("reject=%v error=%v", tc.reject, err)
			}
			if !bytes.Equal(before, body.Bytes()) {
				t.Fatal("body mutated")
			}
		})
	}
}
