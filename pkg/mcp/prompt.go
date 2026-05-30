package mcp

import (
	"context"
	"fmt"
	"log/slog"

	"teleskopio/pkg/kubeapi"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func LoadPrompts(mcpServer *Server) *Server {
	mcpServer.server.AddPrompt(
		mcp.NewPrompt(
			"pods_diagnosis",
			mcp.WithPromptDescription("Investigating pods issues of the kubernetes cluster"),
			mcp.WithArgument("server",
				mcp.ArgumentDescription("The cluster server endpoint"),
				mcp.RequiredArgument(),
			),
		),
		server.PromptHandlerFunc(mcpServer.podsDiagnosis),
	) // pods_diagnosis
	mcpServer.server.AddPrompt(
		mcp.NewPrompt(
			"nodes_diagnosis",
			mcp.WithPromptDescription("Investigating kubernetes cluster nodes issues"),
			mcp.WithArgument("server",
				mcp.ArgumentDescription("The cluster server endpoint"),
				mcp.RequiredArgument(),
			),
		),
		server.PromptHandlerFunc(mcpServer.nodesDiagnosis),
	) // nodes_diagnosis
	return mcpServer
}

func (s Server) nodesDiagnosis(_ context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	slog.Debug("new prompt call", "prompt", "nodes_diagnosis", "req", request.Params.Arguments)
	server := request.Params.Arguments["server"]
	if server == "" {
		return nil, fmt.Errorf("server are required")
	}
	//nolint:lll
	diagnosisPrompt := `
You're an SRE engineer. Follow these steps to investigate the kubernetes nodes issues and generate report for the user:

1. Fetch nodes resource from the %s server by using api_resources tool with kind Node and empty group
2. Use list_resources tool to fetch Node resources with empty group, request short resources overview.
3. Look for NotReady or Unknown by analize conditions, request full node resources overview of the problem node to analize. If all nodes in Ready status there is no issues.

MemoryPressure: The node is running with low memory.
DiskPressure: The node root disk is running out of space.
PIDPressure: Too many concurrent processes are active on the host.

Return short report for the user.
	`
	return mcp.NewGetPromptResult(
		"Nodes issues",
		[]mcp.PromptMessage{
			mcp.NewPromptMessage(
				mcp.RoleUser,
				mcp.NewTextContent(fmt.Sprintf(diagnosisPrompt, server))),
		},
	), nil
}

func (s Server) podsDiagnosis(_ context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	slog.Debug("new prompt call", "prompt", "nodes_diagnosis", "req", request.Params.Arguments)
	server := request.Params.Arguments["server"]
	if server == "" {
		return nil, fmt.Errorf("server are required")
	}
	//nolint:lll
	diagnosisPrompt := `
You're an SRE engineer. Follow these steps to investigate pods issues and generate report for the user:

1. Fetch pods resource from the %s server by using api_resources tool with kind Pod
2. Use list_resources tool to fetch Pod resources, use empty namespace argument to fetch pods across all namespaces, use field_selector status.phase!=Running to list pods in not Running state, request short resources overview.
3. If any pods returned look for CrashLoopBackOff, ImagePullBackOff, OOMKilled, FailedScheduling, Unhealthy, BackOff pod phase by requesting those full resources to analize.

CrashLoopBackOff: Looking logs for application errors
ImagePullBackOff: Wrong image name/tag or pull secrets
Pending: Insufficient resources, node selector mismatch, PVC not bound
OOMKilled: Container memory limit too low for workload or workload has resource leaking.
FailedMount: Missing ConfigMap, Secret, or PV

Return short report for the user.
	`
	return mcp.NewGetPromptResult(
		"Pods issues",
		[]mcp.PromptMessage{
			mcp.NewPromptMessage(
				mcp.RoleUser,
				mcp.NewTextContent(fmt.Sprintf(diagnosisPrompt, server))),
		},
	), nil
}

type ServerEndpointCompletionProvider struct {
	kapi *kubeapi.KubeAPI
}

func (p *ServerEndpointCompletionProvider) CompletePromptArgument(
	_ context.Context,
	promptName string,
	argument mcp.CompleteArgument,
	_ mcp.CompleteContext,
) (*mcp.Completion, error) {
	switch promptName {
	case "pods_diagnosis", "nodes_diagnosis":
		if argument.Name == "server" {
			servers := []string{}
			for _, ss := range p.kapi.GetClusters() {
				servers = append(servers, ss.Server)
			}
			return &mcp.Completion{
				Values:  servers,
				HasMore: false,
			}, nil
		}
	}
	return &mcp.Completion{Values: []string{}}, nil
}
