package requestaudit

import (
	"strings"
	"testing"
)

func BenchmarkRedactCode(b *testing.B) {
	for _, size := range []struct {
		name string
		n    int
	}{{"200KB", 1500}, {"1MB", 7500}} {
		b.Run(size.name, func(b *testing.B) {
			input := strings.Repeat("// inspect token and key usage; client cache enabled\nconst response = await client.request({model: 'example', input: message});\n", size.n) + "\npassword = secret-value\n"
			b.SetBytes(int64(len(input)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				count := 0
				redact(input, &count)
			}
		})
	}
}

func BenchmarkAssignmentComparison(b *testing.B) {
	input := strings.Repeat("// inspect token and key usage; client cache enabled\nconst response = await client.request({model: 'example', input: message});\n", 7500) + "\npassword=secret-value"
	for _, optimized := range []bool{false, true} {
		name := "original"
		if optimized {
			name = "optimized"
		}
		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(len(input)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				count := 0
				if optimized {
					redactAssignments(input, &count)
				} else {
					assignedSecret.ReplaceAllStringFunc(input, func(string) string { count++; return "[REDACTED]" })
				}
			}
		})
	}
}
