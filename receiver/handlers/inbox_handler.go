package handlers

import (
	"net/http"

	"receiver/database"
	"receiver/repository"

	"github.com/gin-gonic/gin"
)

func Health(c *gin.Context) {

	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})

}

func GetInbox(c *gin.Context) {

	email := c.Param("email")

	messages, err := repository.GetInboxMessages(
		database.Conn,
		email,
	)

	if err != nil {

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})

		return

	}

	c.JSON(http.StatusOK, messages)

}
