// Command hcheck answers one question about a raw HTTP header line: is it
// syntactically valid per RFC 9110, or would a strict recipient reject it?
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

// Result is the outcome of checking a single logical header line. Value and
// Name are only meaningful when Err is nil.
type Result struct {
	Raw   string
	Name  string
	Value string
	Err   error
}

func isStatusLine(line string) bool {
	return strings.HasPrefix(line, "HTTP/")
}

// processLines walks a raw sequence of header lines (as read from stdin or
// given as separate arguments), skipping a leading status line and stopping
// at the blank line that ends a header block. Continuation lines that start
// with a space or tab are obsolete line folding (RFC 7230 3.2.4): rejected
// in strict mode, merged into the previous value in lenient mode.
func processLines(lines []string, lenient bool) []Result {
	var results []Result

	for _, line := range lines {
		if line == "" {
			break
		}

		if len(results) == 0 && isStatusLine(line) {
			continue
		}

		if line[0] == ' ' || line[0] == '\t' {
			if len(results) == 0 {
				results = append(results, Result{Raw: line, Err: errors.New("continuation line has no preceding header to fold into")})
				continue
			}
			if !lenient {
				results = append(results, Result{Raw: line, Err: errors.New("obsolete line folding (RFC 7230 3.2.4); pass --lenient to allow it")})
				continue
			}
			prev := &results[len(results)-1]
			if prev.Err == nil {
				prev.Value = prev.Value + " " + strings.TrimSpace(line)
				prev.Raw = prev.Raw + "\n" + line
			}
			continue
		}

		name, value, err := ParseHeaderLine(line, lenient)
		results = append(results, Result{Raw: line, Name: name, Value: value, Err: err})
	}

	return results
}

func main() {
	lenient := flag.Bool("lenient", false, "relax RFC 9110 grammar to match what real clients and servers tolerate")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `hcheck: is this an RFC-valid HTTP header field?

Usage:
  hcheck 'Header-Name: value' ['Another: value' ...]
  curl -D - https://example.com/ | hcheck
  hcheck --lenient < headers.txt

By default hcheck is strict: it rejects anything that isn't legal per
RFC 9110, including bytes outside ASCII and whitespace before the colon.
Pass --lenient to accept the wider range of malformed input that browsers
and curl tolerate in practice.

Flags:
`)
		flag.PrintDefaults()
	}
	flag.Parse()

	var lines []string
	if flag.NArg() > 0 {
		lines = flag.Args()
	} else {
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			fmt.Fprintln(os.Stderr, "hcheck: reading stdin:", err)
			os.Exit(2)
		}
	}

	if len(lines) == 0 {
		fmt.Fprintln(os.Stderr, "hcheck: no input; pass header lines as arguments or pipe them on stdin")
		os.Exit(2)
	}

	results := processLines(lines, *lenient)

	failed := 0
	for _, r := range results {
		if r.Err != nil {
			failed++
			fmt.Printf("FAIL  %s\n      %v\n", r.Raw, r.Err)
			continue
		}
		fmt.Printf("OK    %s: %s\n", r.Name, r.Value)
	}

	if failed > 0 {
		os.Exit(1)
	}
}
