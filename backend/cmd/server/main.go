// Command server boots the Forgify backend: logger, DB, HTTP router with
// middleware chain, and graceful shutdown.
//
// Command server 启动 Forgify 后端：logger、DB、带中间件链的 HTTP 路由、优雅关闭。
//
// TODO: audit all import aliases per S13 (<name><role> convention) after
// the chat infra refactor is fully complete.
// TODO: 重构完成后按 S13（<name><role> 命名约定）统一审计所有 import 别名。
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

	agentapp "github.com/sunweilin/forgify/backend/internal/app/agent"
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
	"github.com/sunweilin/forgify/backend/internal/infra/events/memory"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
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
	dataDir := flag.String("data-dir", "", "Data directory (empty = os.TempDir)")
	dev := flag.Bool("dev", false, "Development mode (colored console logs + /dev/* routes)")
	collectionsDir := flag.String("collections-dir", "../testend/collections", "Path to YAML test collections (dev mode)")
	integrationDir := flag.String("integration-dir", "../testend", "Path to testend/ directory served at /dev/static/ (dev mode)")
	flag.Parse()

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
	defer log.Sync() //nolint:errcheck

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

	if err := db.Migrate(gdb,
		&apikeydomain.APIKey{},
		&modeldomain.ModelConfig{},
		&convdomain.Conversation{},
		&chatdomain.Message{},
		&chatdomain.Block{}, // message_blocks table (chat infra refactor)
		&chatdomain.Attachment{},
		&tooldomain.Tool{},
		&tooldomain.ToolVersion{},
		&tooldomain.ToolTestCase{},
		&tooldomain.ToolRunHistory{},
		&tooldomain.ToolTestHistory{},
	); err != nil {
		log.Error("migrate db", zap.Error(err))
		os.Exit(1)
	}

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
		apikeyapp.NewHTTPTester(nil),
		log,
	)

	modelService := modelapp.NewService(modelstore.New(gdb), log)
	convService := convapp.NewService(convstore.New(gdb), log)

	llmFactory := llminfra.NewFactory()

	// toolLLMClient satisfies toolapp.LLMClient for GenerateTestCases
	// (non-streaming JSON calls only).
	//
	// toolLLMClient 满足 toolapp.LLMClient 接口，仅用于 GenerateTestCases
	// 的非流式 JSON 调用。
	toolLLM := &toolLLMClientAdapter{
		picker:  modelService,
		keys:    apikeyService,
		factory: llmFactory,
	}
	toolService := toolapp.NewService(
		toolstore.New(gdb),
		sandbox.New("python3"),
		toolLLM,
		log,
	)

	chatRepo := chatstore.New(gdb)
	eventsBridge := memory.NewBridge(log)
	chatService := chatapp.NewService(
		chatRepo,
		convstore.New(gdb),
		modelService,
		apikeyService,
		llmFactory,
		eventsBridge,
		*dataDir,
		log,
	)

	forgeTools := agentapp.ForgeTools(
		toolService,
		chatRepo,
		modelService,
		apikeyService,
		llmFactory,
		eventsBridge,
	)
	webTools := agentapp.WebTools()
	allTools := append(forgeTools, webTools...)
	allTools = append(allTools, agentapp.SystemTools()...)
	chatService.SetTools(allTools)

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
		Tools:               allTools,
		DB:                  gdb,
		LogBroadcaster:      broadcaster,
		CollectionsDir:      *collectionsDir,
		IntegrationDir:      *integrationDir,
		Port:                actualPort,
	})

	srv := &http.Server{
		Handler:     handler,
		ReadTimeout: 15 * time.Second,
		// WriteTimeout=0: SSE streams may run for minutes.
		// WriteTimeout=0：SSE 流可能持续几分钟。
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

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown", zap.Error(err))
	}
}

// toolLLMClientAdapter satisfies toolapp.LLMClient using infra/llm.
// Used only for non-streaming calls (GenerateTestCases).
//
// toolLLMClientAdapter 用 infra/llm 满足 toolapp.LLMClient 接口，
// 仅用于非流式调用（GenerateTestCases）。
type toolLLMClientAdapter struct {
	picker  modeldomain.ModelPicker
	keys    apikeydomain.KeyProvider
	factory *llminfra.Factory
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
	client, baseURL, err := c.factory.Build(llminfra.Config{
		Provider: provider, ModelID: modelID,
		Key: creds.Key, BaseURL: creds.BaseURL,
	})
	if err != nil {
		return "", fmt.Errorf("toolLLMClient: build client: %w", err)
	}
	return llminfra.Generate(ctx, client, llminfra.Request{
		ModelID: modelID, Key: creds.Key, BaseURL: baseURL,
		Messages: []llminfra.LLMMessage{{Role: llminfra.RoleUser, Content: prompt}},
	})
}
