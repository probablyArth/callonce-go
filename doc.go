// Package callonce provides request-scoped call coalescing and memoization for Go.
//
// In fan-out architectures — BFF layers, API gateways, GraphQL servers — a single
// HTTP request often spawns multiple goroutines that independently fetch the same
// downstream resource. callonce ensures the function is called exactly once per
// unique input, with the result shared across all concurrent callers and cached
// for the lifetime of the request.
//
// Unlike [golang.org/x/sync/singleflight], which deduplicates only during an
// in-flight call and then forgets the result, callonce persists successful results
// for the full request lifetime. Unlike a global cache, it requires no eviction
// policy — the cache is tied to the context and discarded when the request ends.
//
// # Behavior
//
// Concurrent callers for the same key and identifier share a single in-flight
// call. The first caller executes the function; others block and receive the
// same result. On success, the result is cached — subsequent calls return it
// immediately without executing the function again. On failure, the error is
// returned to all concurrent waiters but is not cached, allowing retries.
//
// If the context has no cache attached, [Get] calls the function directly,
// providing graceful degradation without panicking or requiring setup.
//
// # Usage
//
// Define typed keys once at package level with [NewKey], attach a cache at the
// top of your handler or middleware with [WithCache], then call [Get] anywhere
// downstream:
//
//	var userKey = callonce.NewKey[*User]("user")
//
//	func MyMiddleware(next http.Handler) http.Handler {
//	    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//	        ctx := callonce.WithCache(r.Context())
//	        next.ServeHTTP(w, r.WithContext(ctx))
//	    })
//	}
//
//	func GetUser(ctx context.Context, userID string) (*User, error) {
//	    return callonce.Get(ctx, fetchUserFromDB, callonce.L(userKey, userID))
//	}
//
// No matter how many goroutines call GetUser with the same userID within a
// single request, fetchUserFromDB is called at most once.
package callonce
