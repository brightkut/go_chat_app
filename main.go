package main

import (
	"log"

	"github.com/gofiber/fiber/v2"
)

func main() {
	app := fiber.New()

	app.Get("/", hello)

	log.Fatal(app.Listen(":8088"))
}

func hello(c *fiber.Ctx) error {
	return c.SendString("Hello Chat App")
}
