// Package naming holds shared validation for resource identifiers.
// Keep hyphen slug regexp in sync with modules/web/src/utils/hyphenSlug.ts (HYPHEN_SLUG_PATTERN).
package naming

import (
	"fmt"
	"regexp"
	"strings"
)

var hyphenSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// Max lengths align with gorm column sizes on Certificate.Name and Role.Name.
const (
	MaxCertificateNameLen = 191
	MaxRoleNameLen        = 64
)

// ValidateHyphenSlug checks trimmed name: non-empty, length, lowercase [a-z0-9] with single-hyphen segments only.
func ValidateHyphenSlug(name string, maxLen int) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if len(name) > maxLen {
		return fmt.Errorf("name exceeds maximum length %d", maxLen)
	}
	if !hyphenSlugPattern.MatchString(name) {
		return fmt.Errorf("name must use only lowercase letters, digits, and hyphens between segments (underscores not allowed)")
	}
	return nil
}
