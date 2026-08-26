package handlers

import (
	"encoding/json"
	"net/http"

	"receiver/database"
	"receiver/naming"
	"receiver/repository"

	"github.com/gin-gonic/gin"
)

// GetNamingRules returns the current naming rules (defaults if unset).
// GET /api/admin/naming-rules
func GetNamingRules(c *gin.Context) {
	rules, err := naming.Parse(repository.GetSetting(database.Conn(), "naming_rules"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rules)
}

// UpdateNamingRules validates and stores new naming rules.
// PUT /api/admin/naming-rules  (body = full rules JSON)
func UpdateNamingRules(c *gin.Context) {
	var rules naming.Rules
	if err := c.ShouldBindJSON(&rules); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON: " + err.Error()})
		return
	}
	// Validate by round-tripping through Parse
	data, _ := json.Marshal(rules)
	if _, err := naming.Parse(string(data)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := repository.SetSetting(database.Conn(), "naming_rules", string(data)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save rules"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Naming rules updated", "rules": rules})
}
