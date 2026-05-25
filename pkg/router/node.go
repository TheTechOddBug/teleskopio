package router

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"teleskopio/pkg/model"
	"time"

	"github.com/gin-gonic/gin"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubectl/pkg/drain"
)

func (r *Route) NodeOperation(c *gin.Context) {
	var req model.NodeOperation
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("parsing", "err", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	if err := r.kapi.NodeOperation(c.Request.Context(), req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": ""})
}

func (r *Route) NodeDrain(c *gin.Context) {
	var req model.NodeDrain
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("parsing", "err", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	node, err := r.GetCluster(req.Server).Typed.CoreV1().Nodes().Get(context.TODO(), req.ResourceName, metav1.GetOptions{})
	if err != nil {
		slog.Error("get node", "err", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	drainer := &drain.Helper{
		Ctx:                 context.TODO(),
		Client:              r.GetCluster(req.Server).Typed,
		Force:               req.DrainForce,
		IgnoreAllDaemonSets: req.IgnoreAllDaemonSets,
		DeleteEmptyDirData:  req.DeleteEmptyDirData,
		Timeout:             time.Duration(req.DrainTimeout) * time.Second,
		Out:                 os.Stdout,
		ErrOut:              os.Stderr,
		OnPodDeletedOrEvicted: func(pod *v1.Pod, usingEviction bool) {
			slog.Debug("Deleted/Evicted pod", "ns", pod.Namespace, "pod", pod.Name, "eviction", usingEviction)
			payload, _ := json.Marshal(map[string]interface{}{
				"event":   fmt.Sprintf("drain_%s_%s", req.ResourceName, req.ResourceUID),
				"payload": map[string]any{"pod": pod.Name, "ns": pod.Namespace, "eviction": usingEviction},
			})
			r.hub.Broadcast(payload)
		},
	}

	if err := drain.RunCordonOrUncordon(drainer, node, true); err != nil {
		slog.Error("run cordon or uncordon", "err", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	if err := drain.RunNodeDrain(drainer, req.ResourceName); err != nil {
		slog.Error("run eviction", "err", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": node})
}
