package req

import "testing"

func TestLowercaseHTTP1HeadersClone(t *testing.T) {
	tx := NewTransport()
	if tx.LowercaseHTTP1Headers {
		t.Fatal("lowercase option must be opt-in")
	}
	tx.LowercaseHTTP1Headers = true
	if !tx.Clone().LowercaseHTTP1Headers {
		t.Fatal("clone lost lowercase option")
	}
}
