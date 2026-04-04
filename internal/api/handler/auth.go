package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type LoginRequest struct {
	Key string `json:"key"`
}

func LoginHandler(authKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if req.Key != authKey {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid Key"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"token": authKey})
	}
}
