package main

import (
	"strconv"
	"time"
	"log"
	internal "doc_activity/internal"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2/middleware/logger"
)

const PINGDELAY = 1000

func main() {
	tracker := internal.NewActivityTracker()

	app := fiber.New()

	app.Use(
		logger.New(logger.Config{
			Format: "${time} [${ip}]:${port} ${status} - ${method} ${path}\n",
		}),
	)
	group := app.Group("/users_activity")

	group.Use("/ws", func(c *fiber.Ctx) error {
		// IsWebSocketUpgrade returns true if the client
		// requested upgrade to the WebSocket protocol.
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})

	group.Get("/ws", websocket.New(func(c *websocket.Conn) {
		userIdString := c.Query("user_id")
		if userIdString == "" {
			c.WriteJSON(map[string]interface{}{"Error": "user_id required"})
			c.Close()
			return
		}

		userId, err := strconv.Atoi(userIdString)
		if err != nil {
			c.WriteJSON(map[string]interface{}{"Error": "user_id is invalid"})
			c.Close()
			return
		}

		defer c.Close()

		conn := &internal.SyncConn{C: c}

		stopPing := make(chan struct{})
		defer close(stopPing)
		go func(conn *internal.SyncConn, stop chan struct{}) {
			ticker := time.NewTicker(PINGDELAY * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					// Send ping
					err := conn.C.WriteMessage(websocket.TextMessage, []byte("ping"))
					if err != nil {
						log.Println("ping error:", err)
						return
					}
				case <-stop:
					return
				}
			}
		}(conn, stopPing)

		for {
			c.SetReadDeadline(time.Now().Add(time.Second * PINGDELAY * 2))
			_, msg, err := c.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Println("read error:", err)
				} else {
					log.Println("Ping-pong timeout reached for user_id=", userId)
				}
				break // Calls the deferred function, i.e. closes the connection on error
			}

			msg_data := string(msg)
			if msg_data != "pong" {
				log.Println("received msg:", msg_data)
				tracker.AddUser(userId, conn, msg_data)
				defer tracker.RemoveUser(userId)
			}
		}

	}))

	log.Fatal(app.Listen(":5000"))
	tracker.Wait()
}
