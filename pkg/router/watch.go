package router

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"teleskopio/pkg/model"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	w "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
)

func (r *Route) WatchEventsDynamicResource(c *gin.Context) {
	var req model.WatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("parsing", "err", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	apiResourceList, err := r.GetCluster(req.Server).Typed.ServerResourcesForGroupVersion(schema.GroupVersion{
		Group:   req.APIResource.Group,
		Version: req.APIResource.Version,
	}.String())
	if err != nil {
		slog.Error("api list", "err", err.Error(), "req", req)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	for _, r := range apiResourceList.APIResources {
		if r.Kind == req.APIResource.Kind && r.SingularName == strings.ToLower(req.APIResource.Kind) {
			req.APIResource.Resource = r.Name
		}
	}
	gvr := schema.GroupVersionResource{
		Group:    req.APIResource.Group,
		Version:  req.APIResource.Version,
		Resource: req.APIResource.Resource,
	}
	var ri dynamic.ResourceInterface
	if req.Namespace != "" {
		ri = r.GetCluster(req.Server).Dynamic.Resource(gvr).Namespace(req.Namespace)
	} else {
		ri = r.GetCluster(req.Server).Dynamic.Resource(gvr)
	}
	watcherKey := fmt.Sprintf("%s-%s-updated", req.UID, req.Server)
	_, ok := r.watchers[watcherKey]
	if ok {
		slog.Info("watcher exist", "gvr", gvr.String(), "key", watcherKey)
		c.JSON(http.StatusOK, gin.H{"success": ""})
		return
	}
	watchOptions := metav1.ListOptions{ResourceVersion: req.APIResource.ResourceVersion}
	fieldSelector := ""
	if req.APIResource.Group == "" {
		fieldSelector = fmt.Sprintf("involvedObject.uid=%s", req.UID)
	} else {
		fieldSelector = fmt.Sprintf("regarding.uid=%s", req.UID)
	}
	watchOptions.FieldSelector = fieldSelector
	watch, err := ri.Watch(context.TODO(), watchOptions)
	if err != nil {
		slog.Error("watcher", "err", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	ch := watch.ResultChan()
	r.watchers[watcherKey] = watch
	slog.Info("Watching ...", "gvr", gvr.String())
	go func() {
		for event := range ch {
			switch event.Type {
			case w.Added, w.Modified:
				slog.Debug("message received", "gvr", gvr.String(), "watchKey", watcherKey, "type", event.Type)
				payload, _ := json.Marshal(map[string]interface{}{
					"event":   watcherKey,
					"payload": event.Object,
				})
				r.hub.Broadcast(payload)
			case w.Error:
				slog.Error("watching error", "gvr", gvr.String(), "watchKey", watcherKey, "error", event.Object.DeepCopyObject().GetObjectKind())
				delete(r.watchers, watcherKey)
			}
		}
	}()

	c.JSON(http.StatusOK, gin.H{"success": ""})
}

func (r *Route) WatchDynamicResource(c *gin.Context) {
	var req model.WatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Error("parsing", "err", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	apiResourceList, err := r.GetCluster(req.Server).Typed.ServerResourcesForGroupVersion(schema.GroupVersion{
		Group:   req.APIResource.Group,
		Version: req.APIResource.Version,
	}.String())
	if err != nil {
		slog.Error("api list", "err", err.Error(), "req", req)
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	for _, r := range apiResourceList.APIResources {
		if r.Kind == req.APIResource.Kind && r.SingularName == strings.ToLower(req.APIResource.Kind) {
			req.APIResource.Resource = r.Name
		}
	}
	gvr := schema.GroupVersionResource{
		Group:    req.APIResource.Group,
		Version:  req.APIResource.Version,
		Resource: req.APIResource.Resource,
	}
	var ri dynamic.ResourceInterface
	if req.Namespace != "" {
		ri = r.GetCluster(req.Server).Dynamic.Resource(gvr).Namespace(req.Namespace)
	} else {
		ri = r.GetCluster(req.Server).Dynamic.Resource(gvr)
	}
	watcherKey := fmt.Sprintf("%s-%s", req.APIResource.Kind, req.Server)
	watch, err := ri.Watch(context.TODO(), metav1.ListOptions{ResourceVersion: req.APIResource.ResourceVersion})
	if err != nil {
		slog.Error("watcher", "err", err.Error())
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	ch := watch.ResultChan()
	r.watchers[watcherKey] = watch
	slog.Info("Watching ...", "gvr", gvr.String())
	go func() {
		for event := range ch {
			switch event.Type {
			case w.Added, w.Modified:
				slog.Debug("message received", "gvr", gvr.String(), "watchKey", watcherKey, "type", event.Type)
				payload, _ := json.Marshal(map[string]interface{}{
					"event":   fmt.Sprintf("%s-%s-updated", req.APIResource.Kind, req.Server),
					"payload": event.Object,
				})
				r.hub.Broadcast(payload)
			case w.Deleted:
				slog.Debug("message received", "gvr", gvr.String(), "watchKey", watcherKey, "type", event.Type)
				payload, _ := json.Marshal(map[string]interface{}{
					"event":   fmt.Sprintf("%s-%s-deleted", req.APIResource.Kind, req.Server),
					"payload": event.Object,
				})
				r.hub.Broadcast(payload)
			case w.Error:
				slog.Error("watching error", "gvr", gvr.String(), "watchKey", watcherKey, "error", event.Object.DeepCopyObject().GetObjectKind())
				delete(r.watchers, watcherKey)
			}
		}
	}()

	c.JSON(http.StatusOK, gin.H{"success": ""})
}
