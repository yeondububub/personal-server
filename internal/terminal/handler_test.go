package terminal

import (
	"strings"
	"testing"
)

func TestValidateContainerName(t *testing.T) {
	valid := []string{
		"ps-wg-a1b2c3d4e5f6",
		"ps-wg-test_1.2",
		"nginx-proxy-manager",
		"a1b2c3d4e5f6",
	}
	for _, name := range valid {
		if !ValidateContainerName(name) {
			t.Errorf("expected valid container name %q", name)
		}
	}

	invalid := []string{
		"-evil",
		"foo;id",
		"foo|id",
		"../escape",
		"ps-wg-../escape",
		"ps-wg-bad name",
		"",
		strings.Repeat("a", 129),
	}
	for _, name := range invalid {
		if ValidateContainerName(name) {
			t.Errorf("expected invalid container name %q", name)
		}
	}
}
