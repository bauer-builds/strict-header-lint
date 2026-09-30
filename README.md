# strict-header-lint

`hcheck` answers one question: is this HTTP header field actually valid per
RFC 9110, or does it just happen to work because every parser you've tried
is lenient about the same things?

Most HTTP libraries accept far more than the spec allows, because rejecting
input breaks real traffic. That's the right call for a production server,
but it means malformed headers routinely go unnoticed until they hit a
proxy or cache that isn't as forgiving, or until they're the reason two
systems disagree about where a header ends - the root of several
request-smuggling bugs. `hcheck` is for the times you want the strict
answer on purpose: testing your own server's output, or auditing headers
from something you don't control.

## Usage

Check header lines given directly as arguments:

```
$ hcheck 'Content-Type: text/html; charset=utf-8' 'Cache-Control: max-age=3600'
OK    Content-Type: text/html; charset=utf-8
OK    Cache-Control: max-age=3600
```

Pipe a raw response from curl:

```
$ curl -D - -o /dev/null -s https://example.com/ | hcheck
OK    Content-Type: text/html; charset=UTF-8
OK    ETag: "84238dfc8092e5d9c0dac8ef93371a07:1736799080"
...
```

A header with whitespace before the colon is rejected by default, because
RFC 9110 5.1 forbids it specifically to prevent front-end and back-end
servers from disagreeing about which header a stray space belongs to:

```
$ hcheck 'X-Forwarded-For : 10.0.0.1'
FAIL  X-Forwarded-For : 10.0.0.1
      whitespace before the colon after "X-Forwarded-For" is forbidden by RFC 9110 5.1 (a known request-smuggling vector); pass --lenient to allow it
```

Pass `--lenient` to see what a tolerant parser would accept instead of what
the spec technically allows:

```
$ hcheck --lenient 'X-Forwarded-For : 10.0.0.1'
OK    X-Forwarded-For: 10.0.0.1
```

Exit status is `0` when every header checked out, `1` when at least one
failed, and `2` on a usage error (no input, unreadable stdin).

## What "strict" checks

- Field name is a valid RFC 9110 token (letters, digits, and
  `` !#$%&'*+-.^_`|~ ``) - no exceptions, lenient or not.
- No whitespace between the field name and the colon.
- Field value contains only `VCHAR`, space, and horizontal tab - no control
  bytes, ever, and no bytes outside ASCII unless `--lenient` is set.
- No obsolete line folding (a continuation line starting with a space or
  tab); `--lenient` merges it into the previous header's value instead of
  rejecting it.

A handful of headers get an extra, header-specific check on top of the
generic grammar above:

- `Content-Length` must be `1*DIGIT` (RFC 9110 8.6) - no sign, no
  separators, no trailing garbage.
- `Cache-Control` directives must be `token ["=" (token / quoted-string)]`
  (RFC 9111 5.2), and in strict mode the directive name must be one IANA
  has registered; `--lenient` allows extension directives that aren't in
  the list yet.
- `Set-Cookie` must be `cookie-name "=" cookie-value` followed by
  `"; "`-separated attributes (RFC 6265 4.1.1). The name is a token, the
  value is cookie-octets (optionally double-quoted), `Expires` is an
  RFC 1123 date, `Max-Age` starts with a non-zero digit, and `Domain` is a
  dotted hostname with no leading dot. `--lenient` accepts what browsers
  store anyway: bare `;` separators, spaces and commas in values, older
  date formats, `Max-Age=0` and negatives, and a leading dot on `Domain`.

## Building

```
go build -o hcheck .
```

No third-party dependencies; the standard library is enough.

## What this doesn't do yet

`Content-Length`, `Cache-Control`, and `Set-Cookie` get header-specific
checks; most other headers only get the generic field syntax check. There is
no JSON output yet, and duplicate singleton headers are not flagged.
