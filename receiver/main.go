package main

import (
	"log"

	"receiver/api"
	"receiver/cleanup"
	"receiver/database"
	"receiver/lmtp"

	"github.com/joho/godotenv"
)

func main() {

	err := godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}

	err = database.Connect()
	if err != nil {
		log.Fatal(err)
	}

	// Start cleanup worker for expired inboxes
	cleanup.StartCleanupWorker(database.Conn)

	go lmtp.Start()

	r := api.SetupRouter()

	r.Run(":8080")

}
