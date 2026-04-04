package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	summarysvc "codescan/internal/service/summary"
)

func GetStatsHandler(c *gin.Context) {
	list, err := loadTasksForSummary()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load dashboard stats"})
		return
	}

	c.JSON(http.StatusOK, summarysvc.BuildStats(list))
}
