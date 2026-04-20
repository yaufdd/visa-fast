package consent

import "testing"

func TestCurrentReturnsNonEmpty(t *testing.T) {
	c := Current()
	if c.Version == "" {
		t.Error("consent version is empty")
	}
	if len(c.Body) < 500 {
		t.Error("consent body too short, expected full legal text")
	}
}
