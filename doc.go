// Package callonce provides request-scoped call coalescing and memoization.
//
// When a single HTTP request fans out into multiple goroutines that fetch the
// same downstream resource, callonce ensures the function is called once and
// the result is shared. It combines singleflight-style deduplication with a
// per-request cache, all scoped to a context lifetime.
//
// Define typed keys with [NewKey], attach a cache at the top of your HTTP
// handler (or middleware) with [WithCache], then use [Get] anywhere downstream
// to fetch-or-compute values:
//
//	var userKey = callonce.NewKey[*User]("user")
//
//	ctx := callonce.WithCache(r.Context())
//	user, err := callonce.Get(ctx, fetchUser, callonce.L(userKey, userID))
//
// Concurrent callers for the same key and identifier share a single in-flight
// call. Successful results are cached for the lifetime of the context.
// Errors are not cached, so a failed call can be retried.
//
// If the context has no cache, [Get] calls the function directly, providing
// graceful degradation.
package callonce
