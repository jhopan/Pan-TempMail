package handlers

import (
	"net/http"
	"receiver/database"
	"receiver/repository"
	"strconv"

	"github.com/gin-gonic/gin"
)

func DeleteMessage(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid message ID"})
		return
	}

	err = repository.DeleteMessage(database.Conn, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted", "id": id})
}

func DeleteInbox(c *gin.Context) {
	email := c.Param("email")

	err := repository.DeleteInbox(database.Conn, email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted", "email": email})
}
