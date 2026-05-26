package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"teleskopio/pkg/kubeapi"
	"teleskopio/pkg/model"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
)

type Server struct {
	server *server.MCPServer
	kapi   *kubeapi.KubeAPI
}

const requestTimeout = time.Second * 5

func New(version string, kapi *kubeapi.KubeAPI) *Server {
	mcpServer := server.NewMCPServer(
		"teleskopio",
		version,
		server.WithToolCapabilities(true), // Enable tool capabilities
		server.WithIcons(
			mcp.Icon{
				MIMEType: "image/png",
				Src:      fmt.Sprintf("data:image/png;base64,%s", iconData),
			}),
		server.WithLogging(),  // Enable logging
		server.WithRecovery(), // Enable error recovery
	)

	return &Server{
		kapi:   kapi,
		server: mcpServer,
	}
}

func (s *Server) SetupRoutes(router *gin.Engine) *Server {
	for _, method := range []string{http.MethodPost, http.MethodOptions, http.MethodGet, http.MethodDelete} {
		router.Handle(method, "/mcp", gin.WrapH(s.ServeHTTP()))
	}
	return s
}

func (s *Server) ServeHTTP() *server.StreamableHTTPServer {
	return server.NewStreamableHTTPServer(s.server,
		server.WithHeartbeatInterval(30*time.Second), // TODO custom
		server.WithEndpointPath("/mcp"),
		server.WithStreamableHTTPCORS(
			server.WithCORSAllowedOrigins("*"),
			server.WithCORSAllowCredentials(),
			server.WithCORSMaxAge(300),
		),
	)
}

func LoadTools(mcpServer *Server) *Server {
	mcpServer.server.AddTool(
		mcp.NewTool("clusters",
			mcp.WithDescription("Get available kubernetes cluster endpoints"),
		),
		mcpServer.clusters,
	) // clusters
	mcpServer.server.AddTool(
		mcp.NewTool("cluster_version",
			mcp.WithDescription("Get kubernetes cluster version"),
			mcp.WithInputSchema[model.PayloadRequest](),
			mcp.WithOutputSchema[model.ClusterVersion](),
		),
		mcp.NewStructuredToolHandler(mcpServer.clusterVersion),
	) // cluster_version
	mcpServer.server.AddTool(
		mcp.NewTool("api_resources",
			mcp.WithDescription("Get available api resources of the kubernetes cluster"),
			mcp.WithInputSchema[model.PayloadRequest](),
			mcp.WithOutputSchema[model.APIResourceResponse](),
		),
		mcp.NewStructuredToolHandler(mcpServer.apiResources),
	) // api_resources
	mcpServer.server.AddTool(
		mcp.NewTool("list_resources",
			mcp.WithDescription("Get the list of resources by field selector or label selector. Available resource is requested by api_resources tool. An example of resource key to list nodes: {'apiVersion':'v1','group':'','version':'v1','kind':'Node','namespaced':false,'resource':'nodes'}"),
			mcp.WithInputSchema[model.ResourceFilter](),
			mcp.WithOutputSchema[model.ResourceFilterResponse](),
		),
		mcp.NewStructuredToolHandler(mcpServer.listResources),
	) // get_resources

	return mcpServer
}

func (s *Server) clusters(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slog.Debug("new tool call", "tool", "clusters")
	resp, err := mcp.NewToolResultJSON(map[string]any{"clusters": s.kapi.GetClusters()})
	return resp, err
}

func (s *Server) clusterVersion(ctx context.Context, request mcp.CallToolRequest, args model.PayloadRequest) (model.ClusterVersion, error) {
	slog.Debug("new tool call", "tool", "cluster_version", "args", args)
	cv := model.ClusterVersion{}
	ver, err := s.kapi.GetVersion(args)
	if err != nil {
		return cv, err
	}
	cv.Version = ver.GitVersion
	return cv, nil
}

func (s *Server) apiResources(ctx context.Context, request mcp.CallToolRequest, args model.PayloadRequest) (model.APIResourceResponse, error) {
	slog.Debug("new tool call", "tool", "api_resources", "args", args)
	ar := model.APIResourceResponse{}
	if err := args.Validate(); err != nil {
		return ar, err
	}
	apiResources, err := s.kapi.ListResources(args)
	if err != nil {
		return ar, err
	}
	ar.Items = apiResources
	return ar, err
}

func (s *Server) listResources(ctx context.Context, request mcp.CallToolRequest, args model.ResourceFilter) (model.ResourceFilterResponse, error) {
	slog.Debug("new tool call", "tool", "get_resources", "args", args)
	resources := model.ResourceFilterResponse{}
	if err := args.Validate(); err != nil {
		return resources, err
	}
	kapi, err := s.kapi.GetClient(args.Server)
	if err != nil {
		return resources, err
	}
	apiResourceList, err := s.kapi.GetResource(args.Server, args.Resource)
	if err != nil {
		return resources, err
	}
	s.kapi.SetResource(&args.Resource, apiResourceList)
	gvr := args.Resource.GetGVR()

	var ri dynamic.ResourceInterface
	if args.Resource.Namespaced {
		ri = kapi.Dynamic.Resource(gvr).Namespace(args.Namespace)
	} else {
		ri = kapi.Dynamic.Resource(gvr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	opts := metav1.ListOptions{
		FieldSelector: args.FieldSelector,
		LabelSelector: args.LabelSelector,
	}
	items, err := ri.List(ctx, opts)
	if err != nil {
		return resources, err
	}
	for _, o := range items.Items {
		object := o.Object
		// Remove managedFields
		delete(object["metadata"].(map[string]any), "managedFields")
		resources.Items = append(resources.Items, object)
	}
	return resources, nil
}
