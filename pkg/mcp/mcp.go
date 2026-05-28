package mcp

import (
	"fmt"
	"net/http"
	"teleskopio/pkg/kubeapi"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
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
		server.WithLogging(),     // Enable logging
		server.WithRecovery(),    // Enable error recovery
		server.WithCompletions(), // Enable prompt autocomplete
		server.WithPromptCompletionProvider(&ServerEndpointCompletionProvider{kapi: kapi}),
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
