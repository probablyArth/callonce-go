# callonce-go / Echo Example

Demonstrates request-scoped call coalescing with [Echo](https://echo.labstack.com).

## Run

```
go run .
```

Then visit [http://localhost:3000/user/42](http://localhost:3000/user/42). The handler calls `Get` twice with the same key. Check the server logs to confirm `fetchUser` only runs once.
