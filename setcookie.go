package main

import (
	"fmt"
	"strings"
	"time"
)

// Set-Cookie grammar from RFC 6265 section 4.1.1. The server-side grammar is
// much stricter than what the section 5.2 user-agent algorithm accepts, which
// is the gap this check exists to expose: browsers will store cookies that a
// conforming origin server should never have sent.

// Date layouts a user agent accepts for Expires (RFC 6265 5.1.1 is a fuzzy
// token parser; these are the shapes seen in practice). Only the first is
// legal for a server to send.
var (
	cookieDateStrict  = "Mon, 02 Jan 2006 15:04:05 GMT"
	cookieDateLenient = []string{
		"Mon, 02-Jan-2006 15:04:05 GMT",
		"Monday, 02-Jan-06 15:04:05 GMT",
		"Mon, 02-Jan-06 15:04:05 GMT",
		"Mon Jan _2 15:04:05 2006",
	}
)

// cookie-octet = %x21 / %x23-2B / %x2D-3A / %x3C-5B / %x5D-7E: VCHAR without
// DQUOTE, comma, semicolon, and backslash.
func isCookieOctet(b byte) bool {
	return b == 0x21 ||
		(b >= 0x23 && b <= 0x2B) ||
		(b >= 0x2D && b <= 0x3A) ||
		(b >= 0x3C && b <= 0x5B) ||
		(b >= 0x5D && b <= 0x7E)
}

func validateCookieValue(value string, lenient bool) error {
	body := value
	if len(body) >= 2 && body[0] == '"' && body[len(body)-1] == '"' {
		body = body[1 : len(body)-1]
	}
	for i := 0; i < len(body); i++ {
		b := body[i]
		if isCookieOctet(b) {
			continue
		}
		// Lenient mode follows the user-agent view: anything except the
		// attribute separator and control bytes ends up in the cookie jar.
		if lenient && b != ';' && b >= 0x20 && b != 0x7F {
			continue
		}
		return fmt.Errorf("Set-Cookie value %q contains %q at position %d, which is not a cookie-octet (RFC 6265 4.1.1); pass --lenient to allow it", value, b, i)
	}
	return nil
}

// av-octet = any CHAR except CTLs or ";". Used by Path and extension-av.
func validateAVOctets(attr string) error {
	for i := 0; i < len(attr); i++ {
		b := attr[i]
		if b < 0x20 || b == 0x7F || b == ';' {
			return fmt.Errorf("Set-Cookie attribute %q contains control byte 0x%02x at position %d", attr, b, i)
		}
	}
	return nil
}

func validateCookieDomain(domain string, lenient bool) error {
	if lenient {
		domain = strings.TrimPrefix(domain, ".")
	}
	if domain == "" {
		return fmt.Errorf("Set-Cookie Domain attribute is empty")
	}
	for _, label := range strings.Split(domain, ".") {
		if label == "" || len(label) > 63 {
			return fmt.Errorf("Set-Cookie Domain %q has an empty or over-long label; a leading dot is only accepted with --lenient", domain)
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("Set-Cookie Domain label %q starts or ends with a hyphen", label)
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			if !(c == '-' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
				return fmt.Errorf("Set-Cookie Domain %q contains %q, which is not a letter, digit, or hyphen", domain, c)
			}
		}
	}
	return nil
}

func validateCookieMaxAge(v string, lenient bool) error {
	digits := v
	if lenient {
		digits = strings.TrimPrefix(digits, "-")
	}
	if digits == "" {
		return fmt.Errorf("Set-Cookie Max-Age is empty")
	}
	for i := 0; i < len(digits); i++ {
		if digits[i] < '0' || digits[i] > '9' {
			return fmt.Errorf("Set-Cookie Max-Age %q contains %q, which is not a digit", v, digits[i])
		}
	}
	if !lenient && digits[0] == '0' {
		return fmt.Errorf("Set-Cookie Max-Age %q must start with a non-zero digit (RFC 6265 4.1.1); user agents accept 0 and negatives to expire a cookie, pass --lenient to allow them", v)
	}
	return nil
}

func validateCookieExpires(v string, lenient bool) error {
	if _, err := time.Parse(cookieDateStrict, v); err == nil {
		return nil
	}
	if lenient {
		for _, layout := range cookieDateLenient {
			if _, err := time.Parse(layout, v); err == nil {
				return nil
			}
		}
	}
	return fmt.Errorf("Set-Cookie Expires %q is not an RFC 1123 date like %q; pass --lenient to allow the older formats browsers accept", v, cookieDateStrict)
}

// validateSetCookie checks set-cookie-string = cookie-pair *( ";" SP cookie-av ).
func validateSetCookie(value string, lenient bool) error {
	parts := strings.Split(value, ";")

	pair := parts[0]
	eq := strings.IndexByte(pair, '=')
	if eq < 0 {
		return fmt.Errorf("Set-Cookie has no %q in the cookie-pair", "=")
	}
	if !isToken(pair[:eq]) {
		return fmt.Errorf("Set-Cookie cookie-name %q is not a valid token", pair[:eq])
	}
	if err := validateCookieValue(pair[eq+1:], lenient); err != nil {
		return err
	}

	for _, seg := range parts[1:] {
		attr := seg
		if strings.HasPrefix(attr, " ") {
			attr = attr[1:]
		} else if !lenient {
			return fmt.Errorf("Set-Cookie attribute %q must be separated by \"; \" (semicolon then one space); pass --lenient to allow it", strings.TrimSpace(seg))
		}
		if lenient {
			attr = strings.TrimSpace(attr)
		}
		if attr == "" {
			return fmt.Errorf("Set-Cookie has an empty attribute after a semicolon")
		}
		if err := validateAVOctets(attr); err != nil {
			return err
		}

		name, val := attr, ""
		if i := strings.IndexByte(attr, '='); i >= 0 {
			name, val = attr[:i], attr[i+1:]
		}

		var err error
		switch strings.ToLower(name) {
		case "expires":
			err = validateCookieExpires(val, lenient)
		case "max-age":
			err = validateCookieMaxAge(val, lenient)
		case "domain":
			err = validateCookieDomain(val, lenient)
		}
		if err != nil {
			return err
		}
	}

	return nil
}
