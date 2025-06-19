package main

import (
	"log"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func main() {
	app := fiber.New()

	app.Static("/", "./public")

	app.Get("/api/status", hello)

	chatHub := NewHub()
	go chatHub.Run()

	app.Get("/ws", websocket.New(func(c *websocket.Conn) {
		log.Println("WebSocket connection established!")

		clienId := uuid.New().String()
		client := &Client{
			hub:  chatHub,
			conn: c,
			send: make(chan []byte, 256),
			id:   clienId,
		}

		// reigster new client
		chatHub.register <- client

		log.Printf("New client connected with ID: %s", client.id)

		go client.writePump() // Start goroutine to send messages to the client
		client.readPump()
	}))

	log.Fatal(app.Listen(":8088"))
}

func hello(c *fiber.Ctx) error {
	return c.SendString("Hello Chat App")
}
