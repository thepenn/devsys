package naming

import (
	"strings"
	"testing"
)

func TestValidateHyphenSlug(t *testing.T) {
	max := 191
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"ok single", "gitlab", false},
		{"ok hyphen", "gitlab-token", false},
		{"ok multi", "byteplus-docker-registry", false},
		{"reject underscore", "byteplus_docker_registry", true},
		{"reject upper", "GitLab", true},
		{"reject leading hyphen", "-a", true},
		{"reject trailing hyphen", "a-", true},
		{"reject double hyphen", "a--b", true},
		{"reject spaces only", "   ", true},
		{"reject too long", strings.Repeat("a", 192), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHyphenSlug(tt.input, max)
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
		})
	}
}

func TestValidateHyphenSlug_roleMax(t *testing.T) {
	if err := ValidateHyphenSlug(strings.Repeat("a", 65), MaxRoleNameLen); err == nil {
		t.Fatal("expected length error")
	}
	if err := ValidateHyphenSlug("db-readonly", MaxRoleNameLen); err != nil {
		t.Fatal(err)
	}
}
