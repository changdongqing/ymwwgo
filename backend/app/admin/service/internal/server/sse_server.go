package server

import (
	"github.com/tx7do/kratos-bootstrap/bootstrap"
	"github.com/tx7do/kratos-bootstrap/transport/sse"

	sseServer "github.com/tx7do/kratos-transport/transport/sse"

	"go-wind-admin/app/admin/service/internal/service"
)

// NewSseServer creates a new SSE server.
func NewSseServer(
	ctx *bootstrap.Context,
	internalMessageService *service.InternalMessageService,
	aiChatService *service.AiChatService,
	aiAgentService *service.AiAgentService,
) *sseServer.Server {
	cfg := ctx.GetConfig()

	if cfg == nil || cfg.Server == nil || cfg.Server.Sse == nil {
		return nil
	}

	srv := sse.NewSseServer(cfg.Server.Sse,
		sseServer.WithSubscriberFunction(internalMessageService.HandleSubscribe),
		sseServer.WithAuthorizeFunc(internalMessageService.HandleAuthorize),
	)

	internalMessageService.RegisterInternalMessagePublisher(srv)

	// 同一个 sse.Server 实例同时充当 AI 对话 chunk 的发布者（AiChatPublisher 接口）。
	if aiChatService != nil {
		aiChatService.RegisterAiChatPublisher(srv)
	}

	// 以及 AI Agent 运行实时帧的发布者（AiAgentRunPublisher 接口）。
	if aiAgentService != nil {
		aiAgentService.RegisterAiAgentRunPublisher(srv)
	}

	//srv.CreateStream("test")

	return srv
}
