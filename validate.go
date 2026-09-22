package main

import (
	"errors"
	"fmt"
	"strings"
)

// Grammar reference: RFC 9110 section 5 (field-name, field-value, obs-text)
// and RFC 7230 section 3.2.4 (obsolete line folding). Header bytes are
// treated as single-byte Latin-1-ish octets, not UTF-8, per the spec.

func isTChar(b byte) bool {
	switch b {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	}
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

func isVChar(b byte) bool {
	return b >= 0x21 && b <= 0x7E
}

// ValidateFieldName checks a header name against the RFC 9110 token grammar.
// There is no lenient variant: every real-world header name is a valid
// token, so a failure here means the input isn't a header at all.
func ValidateFieldName(name string) error {
	if len(name) == 0 {
		return errors.New("field name is empty")
	}
	for i := 0; i < len(name); i++ {
		if !isTChar(name[i]) {
			return fmt.Errorf("field name contains %q at position %d, which is not a token character (letters, digits, or one of !#$%%&'*+-.^_`|~)", name[i], i)
		}
	}
	return nil
}

// ValidateFieldValue checks a trimmed header value against the RFC 9110
// field-content grammar: VCHAR, SP, HTAB, and (only when lenient) obs-text
// (bytes 0x80-0xFF). Control bytes are never permitted.
func ValidateFieldValue(value string, lenient bool) error {
	for i := 0; i < len(value); i++ {
		b := value[i]
		switch {
		case b == ' ' || b == '\t':
			continue
		case isVChar(b):
			continue
		case b >= 0x80:
			if lenient {
				continue
			}
			return fmt.Errorf("byte 0x%02x at position %d is outside ASCII (obs-text); pass --lenient to allow it", b, i)
		default:
			return fmt.Errorf("control byte 0x%02x at position %d is never permitted in a header value", b, i)
		}
	}
	if len(value) > 0 && (value[0] == ' ' || value[0] == '\t' || value[len(value)-1] == ' ' || value[len(value)-1] == '\t') {
		return errors.New("value has leading or trailing whitespace that survived trimming")
	}
	return nil
}

// ParseHeaderLine splits a raw "Name: value" line and validates both halves.
// Whitespace between the field name and the colon is rejected outright in
// strict mode: RFC 9110 5.1 forbids it specifically because some proxies
// and origin servers disagree on which side of the colon it belongs to,
// which is a known request-smuggling vector.
func ParseHeaderLine(line string, lenient bool) (name, value string, err error) {
	idx := strings.IndexByte(line, ':')
	if idx < 0 {
		return "", "", errors.New("missing colon separating field name from value")
	}

	rawName := line[:idx]
	rest := line[idx+1:]

	trimmedName := strings.TrimRight(rawName, " \t")
	if trimmedName != rawName {
		if !lenient {
			return "", "", fmt.Errorf("whitespace before the colon after %q is forbidden by RFC 9110 5.1 (a known request-smuggling vector); pass --lenient to allow it", trimmedName)
		}
		rawName = trimmedName
	}

	if err := ValidateFieldName(rawName); err != nil {
		return "", "", err
	}

	value = strings.Trim(rest, " \t")
	if err := ValidateFieldValue(value, lenient); err != nil {
		return "", "", err
	}

	return rawName, value, nil
}
