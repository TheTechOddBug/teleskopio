package model

import (
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

type PayloadRequest struct {
	Server string `json:"server,required" jsonschema_description:"the kubernetes cluster endpoint"`
}

type APIResourceRequest struct {
	Server string `json:"server,required" jsonschema_description:"the kubernetes cluster endpoint"`
	Kind   string `json:"kind" jsonschema_description:"filter data by the kind of resource e.g. Pod, Event, Node. if kind is empty all resources will be returned"`
}

func (p *APIResourceRequest) Validate() error {
	return validation.ValidateStruct(p,
		validation.Field(&p.Server, validation.Required),
	)
}

type ResourceFilter struct {
	Server        string      `json:"server,required" jsonschema_description:"the kubernetes cluster endpoint"`
	Namespace     string      `json:"namespace" jsonschema_description:"the namespace of the resource"`
	Full          bool        `json:"full" jsonschema_description:"return short resources overview instead of full representation. Attributes will be returned name, namespace, kind"`
	Resource      APIResource `json:"resource,required" jsonschema_description:"the kubernetes api resource"`
	FieldSelector string      `json:"field_selector" jsonschema_description:"Chain field selectors by using a comma-separated list, chaining acts as a logical AND operator, meaning a resource is only selected if it matches every criteria in the chain. e.g. status.phase=Running,spec.nodeName=worker-1 or metadata.namespace!=default,status.phase!=NotReady"`
	LabelSelector string      `json:"label_selector" jsonschema_description:"Chain label selectors by using a comma-separated list, chaining acts as a logical AND operator, meaning a resource is only selected if it matches every criteria in the chain. e.g. app=frontend,environment=prod"`
}

func (p *ResourceFilter) Validate() error {
	return validation.ValidateStruct(p,
		validation.Field(&p.Server, validation.Required),
	)
}

type APIResourceResponse struct {
	Items []APIResource `json:"items,required" jsonschema_description:"the list of api resources of cluster"`
}

type ResourceFilterResponse struct {
	Items []map[string]any `json:"items,required" jsonschema_description:"the list of requested resource"`
}

type ClusterVersion struct {
	Version string `json:"version" jsonschema_description:"the kubernetes cluster version"`
}

type ClusterResponse struct {
	Server string `json:"server" jsonschema_description:"the kubernetes cluster endpoint"`
}

func (p *PayloadRequest) Validate() error {
	return validation.ValidateStruct(p,
		validation.Field(&p.Server, validation.Required),
	)
}
