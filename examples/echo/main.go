package main

import (
	"fmt"
	"log"
	"net/http"
	"sync/atomic"

	"github.com/probablyarth/callonce-go"

	"github.com/labstack/echo/v4"
)

var fetchCount atomic.Int32

var userKey = callonce.NewKey[string]("user")

func fetchUser(id string) func() (string, error) {
	return func() (string, error) {
		n := fetchCount.Add(1)
		log.Printf("fetchUser(%s) called (total: %d)", id, n)
		return fmt.Sprintf("user-%s", id), nil
	}
}

func main() {
	e := echo.New()

	// Middleware: attach a callonce cache to every request.
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ctx := callonce.WithCache(c.Request().Context())
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})

	e.GET("/user/:id", func(c echo.Context) error {
		ctx := c.Request().Context()
		id := c.Param("id")

		// Both calls share the same cache — fetchUser runs once.
		user1, _ := callonce.Get(ctx, userKey, id, fetchUser(id))
		user2, _ := callonce.Get(ctx, userKey, id, fetchUser(id))

		return c.JSON(http.StatusOK, map[string]any{
			"first_call":  user1,
			"second_call": user2,
			"same_result": user1 == user2,
		})
	})

	e.Logger.Fatal(e.Start(":3000"))
}
