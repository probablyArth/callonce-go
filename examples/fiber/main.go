package main

import (
	"fmt"
	"log"
	"sync/atomic"

	"github.com/probablyarth/callonce-go"

	"github.com/gofiber/fiber/v2"
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
	app := fiber.New()

	// Middleware: attach a callonce cache to every request.
	app.Use(func(c *fiber.Ctx) error {
		ctx := callonce.WithCache(c.UserContext())
		c.SetUserContext(ctx)
		return c.Next()
	})

	app.Get("/user/:id", func(c *fiber.Ctx) error {
		ctx := c.UserContext()
		id := c.Params("id")

		// Both calls share the same cache, so fetchUser runs once.
		user1, _ := callonce.Get(ctx, userKey, id, fetchUser(id))
		user2, _ := callonce.Get(ctx, userKey, id, fetchUser(id))

		return c.JSON(fiber.Map{
			"first_call":  user1,
			"second_call": user2,
			"same_result": user1 == user2,
		})
	})

	log.Fatal(app.Listen(":3000"))
}
