# httpserver — networks course baseline

A deliberately small HTTP/1.1 server in Go: three routes, an explicit router, one
error envelope, and request logging. Everything is standard library.

This is the **baseline** artifact: the starting point for a course project that
takes AI-generated code and hardens it using protocol and security knowledge from
lecture. The gaps listed under [Assumptions & omissions](#assumptions--omissions)
and [Attack surface](#attack-surface) are intentional — they are the work.

## Layout

```
go.mod
main.go               server construction, timeouts, listen on :8080
handlers/
  routes.go           explicit route table; owns 404 and 405
  handlers.go         the three endpoint handlers; body limit
  errors.go           the single error envelope and JSON writers
middleware/
  logging.go          status/bytes capture and one structured log line per request
```

`handlers` owns the HTTP contract. `middleware` owns cross-cutting concerns. `main`
owns process lifecycle and server configuration. `main` is the only package that
knows about `:8080`.

## Build and run

Requires Go 1.22 or newer.

```sh
go build ./...
go vet ./...
go run .
```

The server listens on `:8080` and logs to stderr:

```
2026/11/25 14:39:13 INFO listening addr=:8080
2026/11/25 14:39:13 INFO request method=GET path=/health status=200 bytes=15 duration=0s remote=127.0.0.1:64377
```

> **Windows note.** No binaries are checked in, but Defender's real-time
> protection occasionally quarantines a freshly linked, unsigned Go binary
> (`The file contains a virus or potentially unwanted software`). Linking with
> `go build -ldflags "-s -w" .` avoids the heuristic. It is an antivirus false
> positive on new binaries, not a problem with the code.

## Endpoint contract

| Method | Path      | Success | Body | Errors |
|---|---|---|---|---|
| `GET`  | `/`       | 200 | `ok` (`text/plain; charset=utf-8`) | 405 |
| `GET`  | `/health` | 200 | `{"status":"ok"}` (`application/json`) | 405 |
| `GET`  | `/echo`   | 200 | the `msg` query value, newline-terminated (`text/plain`) | 400 if `msg` is absent; 405 |
| `POST` | `/echo`   | 201 | the request JSON, echoed byte-for-byte (`application/json`) | 400 for invalid or multi-value JSON; 413 over 1 MiB; 405 |
| any    | anything else | — | — | 404 |

Every non-2xx response is the same envelope:

```json
{"error":"method_not_allowed","message":"method not allowed for this path"}
```

`error` is a stable machine-readable token; `message` is for humans and never
contains the requested path or any internal detail.

`405` responses carry `Allow`, sorted so the value is deterministic
(`Allow: GET, POST`).

## Verified response matrix

Run against a live instance; all fifteen rows were checked. On Windows PowerShell
use `curl.exe`, not `curl` (which aliases `Invoke-WebRequest`).

| # | Command | Status | Body |
|---|---|---|---|
| 1 | `curl.exe -i http://localhost:8080/` | 200 | `ok` |
| 2 | `curl.exe -i http://localhost:8080/health` | 200 | `{"status":"ok"}` |
| 3 | `curl.exe -i "http://localhost:8080/echo?msg=hello"` | 200 | `hello` |
| 4 | `curl.exe -i "http://localhost:8080/echo?msg="` | 200 | *(empty line)* |
| 5 | `curl.exe -i http://localhost:8080/echo` | 400 | `missing_query_parameter` |
| 6 | `curl.exe -i -X POST -d '{"a":1,"b":[2,3]}' http://localhost:8080/echo` | 201 | `{"a":1,"b":[2,3]}` |
| 7 | `curl.exe -i -X POST -d '{"a":1,' http://localhost:8080/echo` | 400 | `invalid_json` |
| 8 | `curl.exe -i -X POST -d '{"a":1}{"b":2}' http://localhost:8080/echo` | 400 | `invalid_json` (more than one value) |
| 9 | `curl.exe -i -X POST --data-binary "@big.json" http://localhost:8080/echo` | 413 | `payload_too_large` |
| 10 | `curl.exe -i http://localhost:8080/nope` | 404 | `not_found` |
| 11 | `curl.exe -i -X DELETE http://localhost:8080/health` | 405, `Allow: GET` | `method_not_allowed` |
| 12 | `curl.exe -i -X POST http://localhost:8080/` | 405, `Allow: GET` | `method_not_allowed` |
| 13 | `curl.exe -i -X PATCH http://localhost:8080/echo` | 405, `Allow: GET, POST` | `method_not_allowed` |
| 14 | `curl.exe -I http://localhost:8080/` | 405, `Allow: GET` | *(see omissions: HEAD is not supported)* |
| 15 | `curl.exe -i "http://localhost:8080/echo?msg=a%0d%0aInjected"` | 200 | `a` + CRLF + `Injected` |

For row 9, create a body over 1 MiB:

```powershell
'{"pad":"' + ('x' * 1200000) + '"}' | Set-Content -NoNewline big.json
```

Row 14 uses `-I` (a `HEAD` request) rather than `-i`; rows 11–14 are worth reading
with `-i` so the `Allow` header is visible.

## Observed framing behavior

Raw TCP probes (a throwaway `TcpClient` script, not checked in). These rows matter
more than they look: they are the baseline that any replacement parser has to
match or beat.

| Probe | Observed |
|---|---|
| `Content-Length` **and** `Transfer-Encoding` present | `400 Bad Request` |
| Two `Content-Length` headers, different values | `400 Bad Request` |
| Two `Content-Length` headers, identical values | `201 Created` — **accepted** |
| `Transfer-Encoding: xchunked` | `501 Not Implemented` |
| `Transfer-Encoding: chunked, identity` | `501 Not Implemented` |
| HTTP/1.1 request with no `Host` | `400 Bad Request` |
| Absolute-form target `GET http://evil.example/health` | `200 OK` — **Host is not validated** |
| Obs-fold continuation line `X-Long: a` / ` b` | `200 OK` — **accepted, folded** |
| CRLF encoded in the path (`/a%0d%0aInjected`) | `404`, and the log line quotes it as `path="/a\r\nInjected"` |
| Space inside the request target | `400 Bad Request` |
| HTTP/0.9-style `GET /` | `400 Bad Request` |
| Lowercase method `get` | `405`, `Allow: GET` (methods are case-sensitive) |

Everything above came from `net/http`'s parser, not from this repository. That is
the point of the two bold rows: RFC 7230 says a server **must** reject an
ambiguous `Content-Length` and **should** reject obs-fold, and here the stdlib
accepts both. Nothing in this codebase would notice if the parser regressed.

## Design notes

**Explicit router instead of `http.ServeMux`.** Go 1.22's mux can return 405 with
`Allow` on its own, but the response body would be plain text, not this project's
envelope. Owning the table means 404 and 405 go through the same `WriteError` as
400 and 413. It also makes the HTTP contract readable in one screen.

**One error helper.** `handlers.WriteError` is the only function that writes a
non-2xx status. It sets the envelope headers before `WriteHeader`, so no error can
be preceded by an implicit 200. `writeJSON` marshals before writing so a failed
marshal cannot leave a partial body behind a sent status line.

**Byte-faithful echo.** `POST /echo` decodes into `json.RawMessage`, so the reply
preserves key order, whitespace and number formatting. Decoding into
`map[string]any` would silently reformat numbers through `float64` and re-sort
keys.

**Bounded body.** `http.MaxBytesReader` enforces the 1 MiB cap while reading, so an
oversized body costs a bounded amount of memory rather than being buffered and
then measured. `errors.As` distinguishes the limit error (413) from invalid JSON
(400).

**Timeouts are security controls.** `ReadHeaderTimeout` bounds slowloris;
`ReadTimeout` bounds the whole request; `WriteTimeout` bounds a slow reader;
`IdleTimeout` bounds a parked keep-alive connection; `MaxHeaderBytes` bounds the
request line plus headers.

## Assumptions & omissions

Deliberate:

- **No graceful shutdown.** `Ctrl+C` terminates the process and drops in-flight
  requests. `srv.Shutdown` with a signal handler is the fix.
- **HEAD is not supported.** RFC 7231 expects `HEAD` wherever `GET` is served;
  this returns `405`. Caches and uptime monitors that probe with `HEAD` will be
  misled.
- **No tests.** `go test ./...` reports `no test files`. The verified matrix above
  was run by hand, not encoded as assertions.
- **No panic recovery middleware.** `net/http` recovers per connection; the client
  sees a dropped connection rather than the JSON envelope.
- **No TLS, auth, rate limiting, metrics, request IDs, CORS, or config files** —
  per the brief.
- **No connection or concurrency cap.** Time bounds each connection; nothing
  bounds how many connections exist at once.
- **Exact path matching only.** No trailing-slash redirect, no `OPTIONS`. Only
  `r.URL.Path` is consulted; `Host` is never inspected.
- **Query strings are not logged.** `path` alone is recorded so credentials placed
  in a query string stay out of the log. The trade-off is that debugging a bad
  `?msg=` requires reproducing it.
- **`?msg=` (present, empty) is a 200**, while a missing `msg` is a 400. Presence
  and emptiness are different inputs and are treated differently.

Inherited from the environment:

- Go's server adds `Date` and does not add a `Server` header. Neither is
  configurable here without a custom header writer.
- Request parsing, chunked decoding, keep-alive bookkeeping and `Expect:
  100-continue` are all `net/http`'s. This repository has no framing code.

## Attack surface

Ordered by severity. Each entry names the trigger, so the list doubles as a test
plan.

**1. Connection and memory exhaustion — High.**
There is no cap on concurrent connections, in-flight requests, or aggregate body
memory. Timeouts bound each connection in *time*, but 800 concurrent connections
that stall for five seconds is still 800 live sockets and goroutines, and 1 MiB ×
unbounded concurrency is unbounded memory.
*Trigger:* open 800 sockets and send `GET / HTTP/1.1\r\n` on each, then stall.
Watch `/health` latency climb. Variant: 1,000 concurrent `POST /echo` requests
with 1 MiB bodies.
*Note:* `MaxHeaderBytes` caps a single request line plus headers at 1 MiB, so
`GET /echo?msg=…` can reflect roughly 1 MiB back per request — a 1:1 amplifier,
dangerous only in aggregate.

**2. Framing correctness is entirely delegated — Medium.**
Nothing here parses `Content-Length` or `Transfer-Encoding`. The rejections in the
table above are Go's, and two RFC-mandated rejections are missing: identical
duplicate `Content-Length` (accepted, 201) and obs-fold (accepted, 200). A
hand-rolled parser in this codebase would inherit neither protection.
*Trigger:* `POST /echo` with `Content-Length: 2` twice, or `X-Long: a` followed by
a line beginning with a space.

**3. `Host` is never validated — Medium.**
Routing keys on `r.URL.Path` only, so any `Host` value reaches the same routes,
and an absolute-form target such as `GET http://evil.example/health HTTP/1.1`
returns `200`.
*Trigger:* send an absolute-form request target, or `Host: evil.example`.
*Impact now:* none — the server generates no links and sits behind no proxy.
*Impact later:* host-header injection, cache poisoning, and poisoned
password-reset links the moment either of those becomes true.

**4. No transport security — Medium (out of scope).**
Plaintext HTTP; headers, bodies and any future credentials are readable on path.
*Trigger:* capture loopback traffic in Wireshark and read the request line.

**5. Unvalidated reflection in `GET /echo` — Low.**
`msg` is echoed verbatim, including CRLF sequences. It cannot reach a header, and
`text/plain` plus `X-Content-Type-Options: nosniff` and an explicit charset keep a
browser from rendering it as HTML, so it is not currently XSS. The reflection
pattern is still there.
*Trigger:* `GET /echo?msg=%3Cscript%3Ealert(1)%3C/script%3E` returns the markup as
text.

**6. No panic recovery — Low.**
`net/http` recovers per connection and closes it; the client gets a truncated
connection instead of the envelope, and the stack trace lands in the log.
*Trigger:* any future nil dereference or out-of-range index in a handler.

**7. Error output is already generic — positive finding.**
404 does not echo the requested path, 413 reveals only the limit, and no response
contains a stack trace or internal identifier. Keep it that way when adding
routes.

## Suggested next phase

Each item maps to a lecture concept and has a measurable before/after:

| Change | Concept | Evidence |
|---|---|---|
| `LimitListener` + per-IP token bucket + in-flight semaphore | resource exhaustion, TCP accept queue | attack 1 stops degrading `/health` |
| Strict framing checks before the handler runs | HTTP/1.1 message framing, smuggling | duplicate CL and obs-fold become 400 |
| `Host` allowlist; reject absolute-form targets | virtual hosting, host-header injection | attack 3 returns 400 |
| Hand-rolled parser behind a flag, tested against the table above | request-line/header parsing, CL vs chunked | same corpus, two implementations |
| `srv.Shutdown` on SIGINT/SIGTERM | connection lifecycle | in-flight request completes during shutdown |
| Table-driven tests for every row in both matrices | verification, regression safety | `go test ./...` green |

The two tables above are the specification for that work: the first must keep
passing unchanged, the second is the list of rows that should flip from accepted
to rejected.
