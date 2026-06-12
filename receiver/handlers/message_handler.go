package handlers

import (
	"net/http"

	"receiver/database"
	"receiver/repository"

	"github.com/gin-gonic/gin"
)

func GetMessage(c *gin.Context) {

	id := c.Param("id")

	message, err := repository.GetMessageByID(
		database.Conn,
		id,
	)

	if err != nil {

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})

		return

	}

	c.JSON(http.StatusOK, message)

}
