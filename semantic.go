package main

import (
	"fmt"
	"strings"
)

// Per-header semantic checks, layered on top of the generic RFC 9110
// field-value grammar in validate.go. A header that passes ValidateFieldValue
// can still be nonsense for its specific type - a Content-Length of "12a"
// is legal field-content but not a legal Content-Length.

// knownCacheDirectives is the IANA HTTP Cache Directive Registry as of
// RFC 9111. It's deliberately not exhaustive of every vendor extension;
// --lenient exists for directives this list hasn't caught up with yet.
var knownCacheDirectives = map[string]bool{
	"max-age":                true,
	"max-stale":              true,
	"min-fresh":              true,
	"no-cache":               true,
	"no-store":               true,
	"no-transform":           true,
	"only-if-cached":         true,
	"must-revalidate":        true,
	"must-understand":        true,
	"public":                 true,
	"private":                true,
	"proxy-revalidate":       true,
	"s-maxage":               true,
	"stale-while-revalidate": true,
	"stale-if-error":         true,
	"immutable":              true,
}

// ValidateSemantics applies header-specific grammar for the headers hcheck
// knows about. Anything else passes through: generic field-value validity
// is all that can be said about it.
func ValidateSemantics(name, value string, lenient bool) error {
	switch {
	case strings.EqualFold(name, "Content-Length"):
		return validateContentLength(value)
	case strings.EqualFold(name, "Cache-Control"):
		return validateCacheControl(value, lenient)
	}
	return nil
}

// RFC 9110 8.6: Content-Length = 1*DIGIT
func validateContentLength(value string) error {
	if value == "" {
		return fmt.Errorf("Content-Length value is empty; RFC 9110 8.6 requires 1*DIGIT")
	}
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return fmt.Errorf("Content-Length value %q contains %q at position %d, which is not a digit (RFC 9110 8.6 requires 1*DIGIT)", value, value[i], i)
		}
	}
	return nil
}

func isToken(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isTChar(s[i]) {
			return false
		}
	}
	return true
}

// RFC 9111 5.2: Cache-Control = #cache-directive, cache-directive = token
// [ "=" ( token / quoted-string ) ]. Directive names are checked against the
// IANA registry in strict mode; --lenient allows anything token-shaped, since
// the registry grows faster than this list does.
func validateCacheControl(value string, lenient bool) error {
	if value == "" {
		return fmt.Errorf("Cache-Control value is empty; expected at least one directive")
	}

	for _, raw := range strings.Split(value, ",") {
		directive := strings.TrimSpace(raw)
		if directive == "" {
			return fmt.Errorf("Cache-Control has an empty directive between commas")
		}

		token := directive
		directiveValue := ""
		hasValue := false
		if idx := strings.IndexByte(directive, '='); idx >= 0 {
			token = directive[:idx]
			directiveValue = directive[idx+1:]
			hasValue = true
		}

		if !isToken(token) {
			return fmt.Errorf("Cache-Control directive name %q is not a valid token", token)
		}

		if hasValue {
			switch {
			case directiveValue == "":
				return fmt.Errorf("Cache-Control directive %q has a trailing %q with no value", token, "=")
			case strings.HasPrefix(directiveValue, `"`):
				if len(directiveValue) < 2 || !strings.HasSuffix(directiveValue, `"`) {
					return fmt.Errorf("Cache-Control directive %q has an unterminated quoted-string value", token)
				}
			case !isToken(directiveValue):
				return fmt.Errorf("Cache-Control directive %q has value %q, which is not a token or quoted-string", token, directiveValue)
			}
		}

		if !lenient && !knownCacheDirectives[strings.ToLower(token)] {
			return fmt.Errorf("Cache-Control directive %q is not in the IANA cache directive registry; pass --lenient to allow extension directives", token)
		}
	}

	return nil
}
