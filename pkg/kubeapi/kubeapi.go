package kubeapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"teleskopio/pkg/config"
	"teleskopio/pkg/model"

	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	k8sYAML "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

type KubeAPI struct {
	clusters []*config.Cluster
}

func New(clusters []*config.Cluster) *KubeAPI {
	return &KubeAPI{clusters: clusters}
}

func (k *KubeAPI) GetClusters() []model.Cluster {
	configs := []model.Cluster{}
	for _, k := range k.clusters {
		configs = append(configs, model.Cluster{Server: k.Address})
	}
	return configs
}

func (k *KubeAPI) GetVersion(req model.PayloadRequest) (*version.Info, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	ver, err := k.getClient(req.Server).Typed.Discovery().ServerVersion()
	if err != nil {
		return nil, err
	}
	return ver, nil
}

func (k *KubeAPI) ListCustomResourceDefinitions(ctx context.Context, req model.PayloadRequest) (*v1.CustomResourceDefinitionList, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	crdList, err := k.getClient(req.Server).APIExtension.ApiextensionsV1().CustomResourceDefinitions().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return crdList, nil
}

func (k *KubeAPI) ListResources(req model.PayloadRequest) ([]model.APIResource, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	discoveryClient := k.getClient(req.Server).Typed.Discovery()
	apiGroupResources, err := discoveryClient.ServerPreferredResources()
	if err != nil {
		return nil, err
	}
	result := []model.APIResource{}
	for _, list := range apiGroupResources {
		gv, err := schema.ParseGroupVersion(list.GroupVersion)
		if err != nil {
			return nil, err
		}
		for _, res := range list.APIResources {
			apiResource := model.APIResource{
				Group:      gv.Group,
				Version:    gv.Version,
				Kind:       res.Kind,
				Resource:   res.Name,
				Namespaced: res.Namespaced,
			}
			apiResource.APIVersion = fmt.Sprintf("%s/%s", gv.Group, gv.Version)
			if gv.Group == "" {
				apiResource.APIVersion = gv.Version
			}
			result = append(result, apiResource)
		}
	}
	return result, nil
}

func (k *KubeAPI) ListDynamicResource(ctx context.Context, req model.ListRequest) ([]unstructured.Unstructured, string, string, error) {
	if err := req.Validate(); err != nil {
		return nil, "", "", err
	}
	apiResourceList, err := k.getClient(req.Server).Typed.ServerResourcesForGroupVersion(schema.GroupVersion{
		Group:   req.APIResource.Group,
		Version: req.APIResource.Version,
	}.String())
	if err != nil {
		return nil, "", "", err
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
		ri = k.getClient(req.Server).Dynamic.Resource(gvr).Namespace(req.Namespace)
	} else {
		ri = k.getClient(req.Server).Dynamic.Resource(gvr)
	}

	list, err := ri.List(ctx, metav1.ListOptions{
		Limit:    req.Limit,
		Continue: req.Continue,
	})
	if err != nil {
		return nil, "", "", err
	}

	for i := range list.Items {
		list.Items[i].SetAPIVersion(req.APIResource.Version)
		if req.APIResource.Group != "" {
			list.Items[i].SetAPIVersion(fmt.Sprintf("%s/%s", req.APIResource.Group, req.APIResource.Version))
		}
		list.Items[i].SetKind(req.APIResource.Kind)
	}
	continueToken, resourceVersion := "", ""
	metadata := list.Object["metadata"].(map[string]interface{})
	if v, ok := metadata["resourceVersion"].(string); ok {
		resourceVersion = v
	}
	if v, ok := metadata["continue"].(string); ok {
		continueToken = v
	}
	return list.Items, continueToken, resourceVersion, nil
}

func (k *KubeAPI) ListEventsDynamicResource(ctx context.Context, req model.ListRequest) ([]unstructured.Unstructured, string, string, error) {
	if err := req.Validate(); err != nil {
		return nil, "", "", err
	}

	apiResourceList, err := k.getClient(req.Server).Typed.ServerResourcesForGroupVersion(schema.GroupVersion{
		Group:   req.APIResource.Group,
		Version: req.APIResource.Version,
	}.String())
	if err != nil {
		return nil, "", "", err
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
		ri = k.getClient(req.Server).Dynamic.Resource(gvr).Namespace(req.Namespace)
	} else {
		ri = k.getClient(req.Server).Dynamic.Resource(gvr)
	}
	fieldSelector := ""
	if req.APIResource.Group == "" {
		fieldSelector = fmt.Sprintf("involvedObject.uid=%s", req.UID)
	} else {
		fieldSelector = fmt.Sprintf("regarding.uid=%s", req.UID)
	}
	listParams := metav1.ListOptions{
		Limit:         req.Limit,
		Continue:      req.Continue,
		FieldSelector: fieldSelector,
	}

	list, err := ri.List(context.TODO(), listParams)
	if err != nil {
		return nil, "", "", err
	}

	for i := range list.Items {
		list.Items[i].SetAPIVersion("%s")
		if req.APIResource.Group != "" {
			list.Items[i].SetAPIVersion(fmt.Sprintf("%s/%s", req.APIResource.Group, req.APIResource.Version))
		}
		list.Items[i].SetKind(req.APIResource.Kind)
	}
	continueToken, resourceVersion := "", ""
	metadata := list.Object["metadata"].(map[string]interface{})
	if v, ok := metadata["resourceVersion"].(string); ok {
		resourceVersion = v
	}
	if v, ok := metadata["continue_"].(string); ok {
		continueToken = v
	}
	return list.Items, continueToken, resourceVersion, nil
}

func (k *KubeAPI) GetRestConfig(req model.HelmRelease) *rest.Config {
	return k.getClient(req.Server).RestConfig
}

func (k *KubeAPI) GetDynamicResource(ctx context.Context, req model.GetRequest) (*unstructured.Unstructured, error) {
	apiResourceList, err := k.getClient(req.Server).Typed.ServerResourcesForGroupVersion(schema.GroupVersion{
		Group:   req.APIResource.Group,
		Version: req.APIResource.Version,
	}.String())
	if err != nil {
		return nil, err
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
		ri = k.getClient(req.Server).Dynamic.Resource(gvr).Namespace(req.Namespace)
	} else {
		ri = k.getClient(req.Server).Dynamic.Resource(gvr)
	}

	res, err := ri.Get(ctx, req.Name, metav1.GetOptions{})
	return res, err
}

func (k *KubeAPI) CreateOrUpdateKubeResource(ctx context.Context, req model.ObjectRequest, op string) (*unstructured.Unstructured, error) {
	// TODO validate
	decoder := k8sYAML.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(req.Yaml)), 1024)
	obj := &unstructured.Unstructured{}
	if err := decoder.Decode(obj); err != nil && err != io.EOF {
		return nil, err
	}

	gvk := obj.GroupVersionKind()

	apiResList, err := k.getClient(req.Server).Typed.ServerResourcesForGroupVersion(schema.GroupVersion{
		Group:   gvk.Group,
		Version: gvk.Version,
	}.String())
	if err != nil {
		return nil, err
	}

	var plural string
	for _, res := range apiResList.APIResources {
		if res.Kind == gvk.Kind {
			plural = res.Name
			break
		}
	}
	if plural == "" {
		return nil, fmt.Errorf("resource kind %s not found in API group %s/%s", gvk.Kind, gvk.Group, gvk.Version)
	}

	gvr := schema.GroupVersionResource{
		Group:    gvk.Group,
		Version:  gvk.Version,
		Resource: plural,
	}

	ns := obj.GetNamespace()
	var ri dynamic.ResourceInterface
	if ns != "" {
		ri = k.getClient(req.Server).Dynamic.Resource(gvr).Namespace(ns)
	} else {
		ri = k.getClient(req.Server).Dynamic.Resource(gvr)
	}

	var result *unstructured.Unstructured
	switch op {
	case "create":
		result, err = ri.Create(ctx, obj, metav1.CreateOptions{})
	case "update":
		result, err = ri.Update(ctx, obj, metav1.UpdateOptions{})
	}
	return result, err
}

func (k *KubeAPI) NodeOperation(ctx context.Context, req model.NodeOperation) error {
	// TODO validate
	apiResourceList, err := k.getClient(req.Server).Typed.ServerResourcesForGroupVersion(schema.GroupVersion{
		Group:   req.APIResource.Group,
		Version: req.APIResource.Version,
	}.String())
	if err != nil {
		return err
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
	ri := k.getClient(req.Server).Dynamic.Resource(gvr)

	payload := []struct {
		Op    string `json:"op"`
		Path  string `json:"path"`
		Value bool   `json:"value"`
	}{{
		Op:    "replace",
		Path:  "/spec/unschedulable",
		Value: req.Cordon,
	}}
	payloadBytes, _ := json.Marshal(payload)

	if _, err := ri.Patch(ctx, req.Name, types.JSONPatchType, payloadBytes, metav1.PatchOptions{}); err != nil {
		return err
	}
	return nil
}

func (k *KubeAPI) TriggerCronjob(ctx context.Context, req model.TriggerCronjob) (string, error) {
	apiResourceList, err := k.getClient(req.Server).Typed.ServerResourcesForGroupVersion(schema.GroupVersion{
		Group:   req.APIResource.Group,
		Version: req.APIResource.Version,
	}.String())
	if err != nil {
		return "", err
	}

	for _, r := range apiResourceList.APIResources {
		if r.Kind == req.APIResource.Kind && r.SingularName == strings.ToLower(req.APIResource.Kind) {
			req.APIResource.Resource = r.Name
		}
	}
	cronJob, err := k.getClient(req.Server).Typed.BatchV1().CronJobs(req.Namespace).Get(ctx, req.Name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	jobSpec := cronJob.Spec.JobTemplate.Spec
	jobName := fmt.Sprintf("%s-manual-%d", req.Name, metav1.Now().Unix())

	_, err = k.getClient(req.Server).Typed.BatchV1().Jobs(req.Namespace).Create(ctx, &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: req.Namespace,
		},
		Spec: jobSpec,
	}, metav1.CreateOptions{})
	if err != nil {
		return "", err
	}
	return jobName, nil
}

func (k *KubeAPI) ScaleResource(ctx context.Context, req model.ResourceOperation) error {
	// TODO validate
	apiResourceList, err := k.getClient(req.Server).Typed.ServerResourcesForGroupVersion(schema.GroupVersion{
		Group:   req.APIResource.Group,
		Version: req.APIResource.Version,
	}.String())
	if err != nil {
		return err
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
	resource, err := k.getClient(req.Server).Dynamic.Resource(gvr).
		Namespace(req.Namespace).
		Get(ctx, req.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	unstr := &unstructured.Unstructured{Object: resource.Object}
	if err := unstructured.SetNestedField(unstr.Object, req.Replicas, "spec", "replicas"); err != nil {
		return err
	}
	if _, err := k.getClient(req.Server).Dynamic.Resource(gvr).
		Namespace(req.Namespace).
		Update(ctx, unstr, metav1.UpdateOptions{}); err != nil {
		return err
	}
	return nil
}

func (k *KubeAPI) DeleteDynamicResources(ctx context.Context, req model.DeleteRequest) error {
	// TODO validate
	apiResourceList, err := k.getClient(req.Server).Typed.ServerResourcesForGroupVersion(schema.GroupVersion{
		Group:   req.APIResource.Group,
		Version: req.APIResource.Version,
	}.String())
	if err != nil {
		return err
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
	if req.APIResource.Namespaced {
		for _, res := range req.Resources {
			if err := k.getClient(req.Server).Dynamic.Resource(gvr).Namespace(res.Namespace).Delete(ctx, res.Name, metav1.DeleteOptions{}); err != nil {
				return err
			}
		}
	} else {
		for _, res := range req.Resources {
			if err := k.getClient(req.Server).Dynamic.Resource(gvr).Delete(ctx, res.Name, metav1.DeleteOptions{}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (k *KubeAPI) getClient(server string) *config.Cluster {
	for _, c := range k.clusters {
		if c.Address == server {
			return c
		}
	}
	return nil
}
