# callonce-go / Echo Example

Demonstrates request-scoped call coalescing and observability with [Echo](https://echo.labstack.com).

## Run

```
go run .
```

## Try it

```
curl http://localhost:3000/user/42
```

### Response

```json
{"first_call":"user-42","same_result":true,"second_call":"user-42"}
```

### Server logs

```
[callonce] MISS  key=string:user id=42
fetchUser(42) called (total: 1)
[callonce] HIT   key=string:user id=42
```

The first `Get` call is a **MISS** — `fetchUser` runs and the result is cached.
The second `Get` call is a **HIT** — the cached value is returned instantly, `fetchUser` is not called again.

Hit the same endpoint again:

```
curl http://localhost:3000/user/42
```

```
[callonce] MISS  key=string:user id=42
fetchUser(42) called (total: 2)
[callonce] HIT   key=string:user id=42
```

A new request gets a fresh cache (scoped to the request context), so `fetchUser` is called again — no stale data across requests.
