package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"codescan/internal/database"
	"codescan/internal/model"
	reportsvc "codescan/internal/service/report"
)

func ExportTaskReportHandler(c *gin.Context) {
	id := c.Param("id")

	var task model.Task
	if err := database.DB.Preload("Stages").First(&task, "id = ?", id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	html, fileName, err := reportsvc.GenerateHTML(task, time.Now())
	if err != nil {
		if err == reportsvc.ErrNoExportableStages {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No completed audit stages available for export"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate report: " + err.Error()})
		return
	}

	fallbackName := strings.TrimSpace(fileName)
	if fallbackName == "" {
		fallbackName = fmt.Sprintf("codescan-report-%s.html", task.ID)
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"; filename*=UTF-8''%s", fallbackName, url.PathEscape(fallbackName)))
	c.Data(http.StatusOK, "text/html; charset=utf-8", html)
}
