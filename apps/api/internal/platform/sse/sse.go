package sse

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func Prepare(c *gin.Context) error {
	header := c.Writer.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	return http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{})
}

func WriteEvent(c *gin.Context, event string, data interface{}) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, payload); err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}

// WriteIDEvent writes one SSE frame carrying the Last-Event-ID cursor
// (the v1 workspace event evt_ id). Native EventSource resumes from it
// automatically; fetch-based transports read it back explicitly.
func WriteIDEvent(c *gin.Context, id, event string, data interface{}) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(c.Writer, "id: %s\nevent: %s\ndata: %s\n\n", id, event, payload); err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}
