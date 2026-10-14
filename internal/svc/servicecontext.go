package svc

import (
	"aiworkc/internal/config"
	"aiworkc/internal/domain"
	"aiworkc/internal/middleware"
	"aiworkc/internal/model"
	"aiworkc/pkg/encrypt"
	"aiworkc/pkg/filesecure"
	"aiworkc/pkg/langchain/callbackx"
	"aiworkc/pkg/langchain/rerank"
	"aiworkc/pkg/langchain/sqlguard"
	"aiworkc/pkg/langchain/tokencost"
	"aiworkc/pkg/langchain/tokenretry"
	"aiworkc/pkg/mongox"
	"aiworkc/pkg/token"
	"context"
	"errors"
	"os"

	"gitee.com/dn-jinmin/tlog"
	openaiSdk "github.com/sashabaranov/go-openai"
	"github.com/tmc/langchaingo/callbacks"
	"github.com/tmc/langchaingo/llms/openai"
	"go.mongodb.org/mongo-driver/mongo"
)

const defaultRootPasswordHash = "$2a$10$ddIvqt7U6zNA9poys.FNCuEZTJY6V.axWy4P7A44TuT9KBegGZlD6"

func DefaultRootPasswordHash() string { return defaultRootPasswordHash }

var ErrAuth = errors.New("不具有权限")

type ServiceContext struct {
	*middleware.Jwt

	Config config.Config

	Mongo *mongo.Database

	model.UserModel
	model.DepartmentModel
	model.DepartmentUserModel
	model.UserTodoModel
	model.TodoModel
	model.ApprovalModel
	model.ChatlogModel
	Whitelist model.WhitelistModel

	LLMs           *tokencost.LLM
	Reranker       rerank.Client
	TokenDaily     *tokencost.DailyLimiter
	AliProxyOpenai *openaiSdk.Client
	OpenaiClient   *openaiSdk.Client
	Callbacks      callbacks.Handler

	ApprovalNotifyCh chan *domain.ApprovalEvent
	ApprovalPush     func(ctx context.Context, evt *domain.ApprovalEvent) error

	Auth func(ctx context.Context) error

	SQLDrafts    *sqlguard.DraftStore
	TokenRetries *tokenretry.Store
	Files        *filesecure.Registry
}

func NewServiceContext(c config.Config) (*ServiceContext, error) {

	mongoDb, err := mongox.MongodbDatabase(&mongox.MongodbConfig{
		User:     c.Mongo.User,
		Password: c.Mongo.Password,
		Hosts:    c.Mongo.Hosts,
		Port:     c.Mongo.Port,
		Database: c.Mongo.Database,
	})
	if err != nil {
		return nil, err
	}

	logger := tlog.NewLogger()
	callbacks := callbacks.CombiningHandler{
		Callbacks: []callbacks.Handler{
			callbackx.NewLogHandler(logger),
			callbackx.NewTitTokenHandle(logger),
		}}

	opts := []openai.Option{
		openai.WithBaseURL(c.Langchain.Url),
		openai.WithToken(c.Langchain.ApiKey),
		openai.WithCallback(callbacks),
		openai.WithEmbeddingModel("text-embedding-v3"),
		openai.WithModel("qwen-turbo"),
	}

	innerLLM, err := openai.New(opts...)
	if err != nil {
		return nil, err
	}
	llm := tokencost.Wrap(innerLLM, "qwen-turbo")

	aliGPTCfg := openaiSdk.DefaultConfig(c.AliGPT.ApiKey)
	aliGPTCfg.BaseURL = c.AliGPT.Url
	aliProxyOpenAi := openaiSdk.NewClientWithConfig(aliGPTCfg)
	openaiGPTCfg := openaiSdk.DefaultConfig(c.Langchain.ApiKey)
	openaiGPTCfg.BaseURL = c.Langchain.Url
	openaiGPT := openaiSdk.NewClientWithConfig(openaiGPTCfg)

	userModel := model.NewUserModel(mongoDb)
	whitelistModel := model.NewWhitelistModel(mongoDb)
	if err := whitelistModel.SeedDefaults(context.Background()); err != nil {
		return nil, err
	}
	svc := &ServiceContext{
		Config: c,
		Jwt:    middleware.NewJwt(c.Jwt.Secret),

		Mongo: mongoDb,

		UserModel:           userModel,
		DepartmentUserModel: model.NewDepartmentUserModel(mongoDb),
		DepartmentModel:     model.NewDepartmentModel(mongoDb),
		UserTodoModel:       model.NewUserTodoModel(mongoDb),
		TodoModel:           model.NewTodoModel(mongoDb),
		ApprovalModel:       model.NewApprovalModel(mongoDb),
		ChatlogModel:        model.NewChatlogModel(mongoDb),
		Whitelist:           whitelistModel,

		LLMs:             llm,
		Reranker:         rerank.New(rerank.Endpoint(c.Langchain.Url, c.Langchain.RerankUrl), c.Langchain.ApiKey, c.Langchain.RerankModel),
		TokenDaily:       tokencost.NewDailyLimiter(tokencost.DailyTokenLimit),
		Callbacks:        callbacks,
		AliProxyOpenai:   aliProxyOpenAi,
		OpenaiClient:     openaiGPT,
		ApprovalNotifyCh: make(chan *domain.ApprovalEvent, 32),
		SQLDrafts:        sqlguard.NewDraftStore(sqlguard.DefaultDraftTTL),
		TokenRetries:     tokenretry.NewStore(tokenretry.DefaultTTL),
		Files:            filesecure.NewRegistry(filesecure.DefaultFileTTL),
		Auth: func(ctx context.Context) error {
			uid := token.GetUId(ctx)
			if uid == "" {
				return ErrAuth
			}

			user, err := userModel.FindOne(ctx, uid)
			if err != nil {
				return err
			}

			if !user.IsSystem {
				return ErrAuth
			}

			return nil
		},
	}

	return svc, initUser(svc)
}

func initUser(svc *ServiceContext) error {
	ctx := context.Background()

	systemUser, err := svc.UserModel.FindSysStemUser(ctx)
	if err != nil && err != model.ErrNotUser {
		return err
	}
	if systemUser != nil {
		return nil
	}

	password := defaultRootPasswordHash
	if bootstrap := os.Getenv("AIWORKC_BOOTSTRAP_PASSWORD"); bootstrap != "" {
		hashed, err := encrypt.GenPasswordHash([]byte(bootstrap))
		if err != nil {
			return err
		}
		password = string(hashed)
	}

	return svc.UserModel.Insert(ctx, &model.User{
		Name:     "root",
		Password: password,
		Status:   0,
		IsSystem: true,
	})
}
