package router

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"teleskopio/pkg/model"
	"time"

	"github.com/gin-gonic/gin"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *Route) GetPodLogs(c *gin.Context) {
	var req model.PodLogRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("parsing", "err", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	podLogOptions := &v1.PodLogOptions{
		TailLines: req.TailLines,
		Container: req.Container,
	}
	logsReq := r.GetCluster(req.Server).Typed.CoreV1().Pods(req.Namespace).GetLogs(req.Name, podLogOptions)
	podLogs, err := logsReq.Stream(context.Background())
	if err != nil {
		slog.Error("get stream", "err", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	defer podLogs.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, podLogs)
	if err != nil {
		slog.Error("copy stream", "err", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	lines := []string{}
	for {
		line, err := buf.ReadString('\n')
		if err == io.EOF {
			break
		}
		lines = append(lines, line)
	}

	c.JSON(http.StatusOK, lines)
}

func (r *Route) StreamPodLogs(c *gin.Context) {
	var req model.PodLogRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("parsing", "err", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	podLogOptions := &v1.PodLogOptions{
		Follow:    true,
		Container: req.Container,
	}
	timeNow := metav1.NewTime(time.Now())
	podLogOptions.SinceTime = &timeNow
	logsReq := r.GetCluster(req.Server).Typed.CoreV1().Pods(req.Namespace).GetLogs(req.Name, podLogOptions)
	podLogs, err := logsReq.Stream(context.Background())
	if err != nil {
		slog.Error("get stream", "err", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	// TODO might be a collision with another server
	podLogsKey := fmt.Sprintf("pod_log_line_%s_%s", req.Name, req.Namespace)
	if _, ok := r.podLogsWatchers[podLogsKey]; ok {
		slog.Info("pod logs exist", "key", podLogsKey)
		c.JSON(http.StatusOK, gin.H{"success": ""})
		return
	}
	r.podLogsWatchers[podLogsKey] = make(chan bool)
	stopAndClean := func() {
		slog.Debug("stop pod logs stream", "pod", podLogsKey)
		delete(r.podLogsWatchers, podLogsKey)
		podLogs.Close()
	}
	cancel := func() bool {
		select {
		case <-r.podLogsWatchers[podLogsKey]:
			return true
		default:
			return false
		}
	}
	go func() {
		defer stopAndClean()
		for cancel() {
			buf := make([]byte, 2000)
			numBytes, err := podLogs.Read(buf)
			if err == io.EOF {
				break
			}
			if numBytes == 0 {
				time.Sleep(time.Second)
				continue
			}
			if err != nil {
				break
			}
			message := string(buf[:numBytes])
			slog.Debug("log line", "line", message, "pod", podLogsKey)
			payload, _ := json.Marshal(map[string]interface{}{
				"event": podLogsKey,
				"payload": map[string]interface{}{
					"container": req.Container,
					"pod":       req.Name,
					"namespace": req.Namespace,
					"line":      message,
				},
			})
			r.hub.Broadcast(payload)
		}
	}()

	c.JSON(http.StatusOK, gin.H{"success": ""})
}

func (r *Route) StopStreamPodLogs(c *gin.Context) {
	var req model.PodLogRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("parsing", "err", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	podLogsKey := fmt.Sprintf("pod_log_line_%s_%s", req.Name, req.Namespace)

	r.podLogsWatchers[podLogsKey] <- true

	c.JSON(http.StatusOK, gin.H{"success": ""})
}
