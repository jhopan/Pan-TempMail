package handlers

import (
	"net/http"
	"os"
	"receiver/database"
	"receiver/repository"
	"strconv"

	"github.com/gin-gonic/gin"
)

func GetAttachment(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid attachment ID"})
		return
	}

	attachment, err := repository.GetAttachmentByID(database.Conn, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Attachment not found"})
		return
	}

	// Check if file exists
	if _, err := os.Stat(attachment.Path); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Attachment file not found"})
		return
	}

	// Set headers for download
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Content-Disposition", "attachment; filename="+attachment.FileName)
	c.Header("Content-Type", attachment.ContentType)

	c.File(attachment.Path)
}

func GetMessageAttachments(c *gin.Context) {
	idStr := c.Param("id")
	messageID, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid message ID"})
		return
	}

	attachments, err := repository.GetAttachmentsByMessageID(database.Conn, messageID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return attachment info (without file data)
	type AttachmentInfo struct {
		ID          int    `json:"id"`
		FileName    string `json:"filename"`
		ContentType string `json:"content_type"`
		Size        int64  `json:"size"`
	}

	var result []AttachmentInfo
	for _, att := range attachments {
		info, err := os.Stat(att.Path)
		size := int64(0)
		if err == nil {
			size = info.Size()
		}

		result = append(result, AttachmentInfo{
			ID:          att.ID,
			FileName:    att.FileName,
			ContentType: att.ContentType,
			Size:        size,
		})
	}

	c.JSON(http.StatusOK, result)
}
