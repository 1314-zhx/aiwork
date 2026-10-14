# aiwork

办公场景的 AI 助手后端（Go）。用户用自然语言处理待办、请假审批、制度问答、预算查询和文件分析。模型走阿里云 DashScope 兼容接口（Qwen），HTTP + SSE；人与人的会话走 WebSocket。

仓库：[github.com/1314-zhx/aiwork](https://github.com/1314-zhx/aiwork)

> 演示项目：分层记忆目前在进程内存中，重启会丢；请勿把真实密钥提交进 Git。

## 亮点

- **意图路由后再干活**：先 Router 判断通道（闲聊 / 待办 / 审批 / 知识库 / 分析），再进入对应 Handler，避免所有问题都塞进同一个超长 Agent。
- **原生 Function Calling，不是文本 ReAct**：办公工具用 `StrictReActAgent` + 自研 `agentx.Run`。待办/审批最多两跳；工具成功后禁止再调；创建类只出确认草稿，点确认才写库。
- **分层会话记忆**：L0 近轮原文，L1/L2 带目录的摘要，办公按目录命中再展开正文；账号事实与对话抽取分开，口头改不了租户身份。
- **SQL 不让模型直接拼**：模型只出 QueryIR（表、列、过滤槽位），服务端白名单编译 + `?` 绑定 + 强制租户条件，确认后再执行。
- **知识库父子块**：子块向量检索（Redis），过召回后精排，再按 id 取父块；双缓冲世代切换，更新时旧索引仍可读。
- **多模态是文本化**：语音 Whisper 转写；PDF/TXT/CSV 抽正文进 UNTRUSTED 帧；图表服务端绘制后以 URL 返回。

## 能力一览

| 通道 | 行为 |
|---|---|
| 闲聊 | 只带近轮记忆直接作答 |
| 待办 | `todo_add` / `todo_find`；创建需用户确认 |
| 审批 | 请假/外出/补卡；**同一用户同时只能有一条占用中的请假**（处理中或已通过） |
| 知识库 | 制度问答、手册更新 |
| 分析 | 预算表查询（IR → 确认 SQL）或本地文件分析 |

对外接口（JWT，登录除外）：`POST /v1/chat/`、`POST /v1/chat/stream`、`POST /v1/chat/confirm`，以及用户/部门/待办/审批/上传/白名单。

## 安全控制

**接入层**

- JWT；对话与上传必须登录。
- 每 IP / 用户 **60 次/分钟** 限流。
- CORS 白名单；问句服务端截断 **500 字**。

**提示词与输出**

- 用户输入、历史、工具结果、SQL 行进 `<<<UNTRUSTED>>>` 帧，并 Sanitize（去 HTML、注入常用结构、限制长度）。
- 回复打码密钥/手机号等；SQL 确认稿对非管理员默认不回显完整 SQL。

**文件**

- 体积 **10MB**；扩展名白名单 + **magic**（PDF/PNG/JPEG/文本），扩展名与内容不一致会拒。
- 落盘路径防 `../` 穿越；知识更新走系统权限校验。

**数据与权限**

- 分析接口有部门/用户白名单；系统账号可绕过。
- SQL：仅 SELECT、表/列/函数白名单、敏感列拦截、租户由服务端注入（模型禁止自己加 user 条件）。
- 只读 MySQL 连接；无业务过滤时限制扫描行数。
- 请假：应用层查占用 + 进程内锁 + Mongo **部分唯一索引**（`userId` + 请假且未结束）。

**Agent**

- 每跳最多一个工具；成功后再调会被 GATE 拒绝。
- 官方 Executor 工具一失败就整轮挂掉；本项目把失败写成 Observation，允许换参重试。

## 成本控制

| 项 | 策略 |
|---|---|
| 日额度 | 约 **40000 token/用户/日**，进门预扣，用完拒绝 |
| 单次预算 | 闲聊约 3000；工具路径约 8000；分析约 4000 |
| 生成帽 | 路由 256、Agent 首跳 512 / 后续 400、摘要 400、修格式 256 |
| 记忆 | 进模型前压到约 1500 token；L0/L1/L2 分层压缩 |
| 调用次数 | 待办/审批 **maxIter=2**；分析多次 LLM 但无 ReAct 空转 |
| 执行前确认 | SQL / 创建待办 / 请假先出草稿，避免误跑贵查询或写库 |
| 超限 | 单次打满可发重试票（提高本次帽），不是无限重试 |

`tokencost.Wrap` 在每次 `GenerateContent` 前 `Guard` 估算值，回来按 usage 入账。路由失败或额度不足不会把半成品写入对话记忆。

## 技术栈

Go、Gin、LangChainGo、MongoDB（业务）、MySQL（预算只读）、Redis / RedisSearch（知识向量，需 RediSearch/Redis Stack）。

## 快速开始

依赖：Go 1.25+、MongoDB、MySQL（budget 表）、Redis Stack（知识库）。

```bash
cp etc/api.yaml.example etc/api.yaml
# 填写 Langchain.ApiKey、Jwt.Secret、数据库连接
go run . -f ./etc/api.yaml
```

- HTTP：`http://127.0.0.1:8888`
- WebSocket：`0.0.0.0:9000`
- 登录：`POST /v1/user/login`

知识库检索需要带 RediSearch 的 Redis；Windows 普通 Redis 5 没有该模块时，闲聊/待办/审批仍可用。

## 仓库说明

- `etc/api.yaml` 含密钥，已加入 `.gitignore`；只提交 `etc/api.yaml.example`。
- `vendor/` 不入库，请 `go mod download`。
- 分层记忆、日额度、SQL 草稿、文件注册表均为进程内存储，多实例不会共享。
