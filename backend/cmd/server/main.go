// Command server boots the Forgify backend: logger, DB, HTTP router with
// middleware chain, and graceful shutdown. Per-domain handlers and
// services wire in through router.Deps as Phase 2 progresses.
//
// Command server 启动 Forgify 后端：logger、DB、带中间件链的 HTTP 路由、
// 优雅关闭。各 domain 的 handler 和 service 随 Phase 2 推进通过
// router.Deps 接入。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/cloudwego/eino/schema"
	agentpkg "github.com/sunweilin/forgify/backend/internal/app/agent"
	apikeyapp "github.com/sunweilin/forgify/backend/internal/app/apikey"
	chatapp "github.com/sunweilin/forgify/backend/internal/app/chat"
	convapp "github.com/sunweilin/forgify/backend/internal/app/conversation"
	modelapp "github.com/sunweilin/forgify/backend/internal/app/model"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	tooldomain "github.com/sunweilin/forgify/backend/internal/domain/tool"
	infracrypto "github.com/sunweilin/forgify/backend/internal/infra/crypto"
	"github.com/sunweilin/forgify/backend/internal/infra/db"
	einoinfra "github.com/sunweilin/forgify/backend/internal/infra/eino"
	"github.com/sunweilin/forgify/backend/internal/infra/events/memory"
	"github.com/sunweilin/forgify/backend/internal/infra/logger"
	"github.com/sunweilin/forgify/backend/internal/infra/sandbox"
	apikeystore "github.com/sunweilin/forgify/backend/internal/infra/store/apikey"
	chatstore "github.com/sunweilin/forgify/backend/internal/infra/store/chat"
	convstore "github.com/sunweilin/forgify/backend/internal/infra/store/conversation"
	modelstore "github.com/sunweilin/forgify/backend/internal/infra/store/model"
	toolstore "github.com/sunweilin/forgify/backend/internal/infra/store/tool"
	"github.com/sunweilin/forgify/backend/internal/transport/httpapi/router"
)

func main() {
	port := flag.Int("port", 0, "HTTP port (0 = pick a free port, print it)")
	dataDir := flag.String("data-dir", "", "Data directory (empty = in-memory SQLite)")
	dev := flag.Bool("dev", false, "Development mode (colored console logs + /dev/* routes)")
	collectionsDir := flag.String("collections-dir", "../testend/collections", "Path to YAML test collections (dev mode)")
	integrationDir := flag.String("integration-dir", "../testend", "Path to testend/ directory served at /dev/static/ (dev mode)")
	flag.Parse()

	// In dev mode, wire a LogBroadcaster as a second Zap core so that all
	// log entries are also streamed to the /dev/logs SSE endpoint.
	//
	// dev 模式下，把 LogBroadcaster 作为第二个 Zap core 接入，让所有日志
	// 同时流向 /dev/logs SSE 端点。
	var broadcaster *logger.LogBroadcaster
	var logExtras []zapcore.Core
	if *dev {
		broadcaster = logger.NewLogBroadcaster()
		logExtras = []zapcore.Core{broadcaster}
	}

	log, err := logger.New(*dev, logExtras...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync() //nolint:errcheck // flush on exit; error is noise

	gdb, err := db.Open(db.Config{DataDir: *dataDir})
	if err != nil {
		log.Error("open db", zap.Error(err))
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(gdb); err != nil {
			log.Warn("close db", zap.Error(err))
		}
	}()

	// Domain tables — append new GORM models here when adding a Phase.
	// Domain 表——新增 Phase 时把 GORM model 追加到这里。
	if err := db.Migrate(gdb,
		&apikeydomain.APIKey{},
		&modeldomain.ModelConfig{},
		&convdomain.Conversation{},
		&chatdomain.Message{},
		&chatdomain.Attachment{},
		// Phase 3: tool domain
		&tooldomain.Tool{},
		&tooldomain.ToolVersion{},
		&tooldomain.ToolTestCase{},
		&tooldomain.ToolRunHistory{},
		&tooldomain.ToolTestHistory{},
	); err != nil {
		log.Error("migrate db", zap.Error(err))
		os.Exit(1)
	}

	// apikey stack: machine-fingerprint-derived AES-GCM encryptor, HTTP
	// connectivity tester, GORM-backed store, orchestrating Service.
	// Fingerprint failure is fatal — sharing a fallback key across users
	// would be a critical security hole (see infra/crypto.ErrNoFingerprint).
	//
	// apikey 栈：基于机器指纹派生的 AES-GCM encryptor、HTTP 连通性 tester、
	// GORM store、Service 编排。指纹获取失败直接退出——共享 fallback 密钥
	// 等于严重安全漏洞（见 infra/crypto.ErrNoFingerprint）。
	fingerprint, err := infracrypto.MachineFingerprint()
	if err != nil {
		log.Error("machine fingerprint", zap.Error(err))
		os.Exit(1)
	}
	encryptor, err := infracrypto.NewAESGCMEncryptor(infracrypto.DeriveKey(fingerprint))
	if err != nil {
		log.Error("build encryptor", zap.Error(err))
		os.Exit(1)
	}
	apikeyService := apikeyapp.NewService(
		apikeystore.New(gdb),
		encryptor,
		apikeyapp.NewHTTPTester(nil), // default 10s timeout / 默认 10s 超时
		log,
	)

	modelService := modelapp.NewService(modelstore.New(gdb), log)
	convService := convapp.NewService(convstore.New(gdb), log)

	// Phase 3: tool service.
	// toolLLMClient adapts the existing model factory to the thin LLMClient
	// interface used by Service.GenerateTestCases (non-streaming JSON calls).
	//
	// toolLLMClient 把已有 model factory 适配成 toolapp.LLMClient 接口
	// （供 GenerateTestCases 使用的非流式 JSON 调用）。
	einoFactory := einoinfra.NewDefaultFactory()
	toolLLM := &toolLLMClientAdapter{
		picker:  modelService,
		keys:    apikeyService,
		factory: einoFactory,
	}
	toolService := toolapp.NewService(
		toolstore.New(gdb),
		sandbox.New("python3"),
		toolLLM,
		log,
	)

	eventsBridge := memory.NewBridge(log)
	chatService := chatapp.NewService(
		chatstore.New(gdb),
		convstore.New(gdb),
		modelService,
		apikeyService,
		einoFactory,
		eventsBridge,
		*dataDir,
		log,
	)

	// Inject all system tools into the ReAct Agent:
	//   - ForgeTools: user tool library (search/get/create/edit/run)
	//   - WebTools:   web_search + fetch_url
	//   - SystemTools: file I/O, shell, python, datetime
	//
	// 把所有 system tool 注入 ReAct Agent：
	//   - ForgeTools：用户工具库（search/get/create/edit/run）
	//   - WebTools：web_search + fetch_url
	//   - SystemTools：文件读写、shell、python、datetime
	forgeTools := agentpkg.ForgeTools(
		toolService,
		chatstore.New(gdb),
		modelService,
		apikeyService,
		einoFactory,
		eventsBridge,
	)
	webTools, err := agentpkg.WebTools(context.Background())
	if err != nil {
		log.Warn("web tools unavailable", zap.Error(err))
		webTools = nil
	}
	allTools := append(forgeTools, webTools...)
	allTools = append(allTools, agentpkg.SystemTools()...)
	chatService.SetTools(allTools)

	// Listen first so we know the actual port before building router.Deps.
	// 先监听，才能在构建 router.Deps 前知道实际端口。
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		log.Error("listen", zap.Error(err))
		os.Exit(1)
	}
	actualPort := listener.Addr().(*net.TCPAddr).Port

	// Electron reads this line from stdout to discover the port.
	// Electron 从 stdout 读取此行发现端口。
	fmt.Printf("BACKEND_PORT=%d\n", actualPort)

	handler := router.New(router.Deps{
		Log:                 log,
		APIKeyService:       apikeyService,
		ModelService:        modelService,
		ConversationService: convService,
		ToolService:         toolService,
		ChatService:         chatService,
		EventsBridge:        eventsBridge,
		Dev:                 *dev,
		DB:                  gdb,
		LogBroadcaster:      broadcaster,
		CollectionsDir:      *collectionsDir,
		IntegrationDir:      *integrationDir,
		Port:                actualPort,
	})

	srv := &http.Server{
		Handler:     handler,
		ReadTimeout: 15 * time.Second,
		// WriteTimeout=0 because SSE streams may run for minutes.
		// WriteTimeout=0，因为 SSE 流可能持续几分钟。
		IdleTimeout: 60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("serve", zap.Error(err))
			stop()
		}
	}()
	log.Info("backend started", zap.Int("port", actualPort), zap.Bool("dev", *dev))

	<-ctx.Done()
	log.Info("shutdown requested")

	// Give in-flight requests up to 5s before forcing shutdown.
	// 给进行中的请求最多 5 秒完成，之后强制关闭。
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown", zap.Error(err))
	}
}

// toolLLMClientAdapter satisfies toolapp.LLMClient using the existing model
// factory + key provider stack. Used only for non-streaming JSON calls
// (GenerateTestCases). Streaming calls live in app/agent/forge.go.
//
// toolLLMClientAdapter 用已有 model factory + key provider 栈满足
// toolapp.LLMClient 接口。仅用于非流式 JSON 调用（GenerateTestCases）。
// 流式调用在 app/agent/forge.go。
type toolLLMClientAdapter struct {
	picker  modeldomain.ModelPicker
	keys    apikeydomain.KeyProvider
	factory einoinfra.ChatModelFactory
}

func (c *toolLLMClientAdapter) Generate(ctx context.Context, prompt string) (string, error) {
	provider, modelID, err := c.picker.PickForChat(ctx)
	if err != nil {
		return "", fmt.Errorf("toolLLMClient: pick model: %w", err)
	}
	creds, err := c.keys.ResolveCredentials(ctx, provider)
	if err != nil {
		return "", fmt.Errorf("toolLLMClient: resolve credentials: %w", err)
	}
	built, err := c.factory.Build(ctx, einoinfra.ModelConfig{
		Provider: provider, ModelID: modelID,
		Key: creds.Key, BaseURL: creds.BaseURL,
	})
	if err != nil {
		return "", fmt.Errorf("toolLLMClient: build model: %w", err)
	}
	msg, err := built.Model.Generate(ctx, []*schema.Message{schema.UserMessage(prompt)})
	if err != nil {
		return "", fmt.Errorf("toolLLMClient: generate: %w", err)
	}
	return msg.Content, nil
}
