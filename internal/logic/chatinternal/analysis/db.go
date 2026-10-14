/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"aiworkc/internal/domain"
	"aiworkc/internal/svc"
	"aiworkc/pkg/audit"
	"aiworkc/pkg/httpx"
	"aiworkc/pkg/langchain"
	"aiworkc/pkg/langchain/defend"
	"aiworkc/pkg/langchain/router"
	"aiworkc/pkg/langchain/sqlguard"
	"aiworkc/pkg/langchain/visual"
	"aiworkc/pkg/token"

	"github.com/segmentio/ksuid"
	"github.com/tmc/langchaingo/callbacks"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/outputparser"
	"github.com/tmc/langchaingo/prompts"
	"github.com/tmc/langchaingo/tools/sqldatabase"
)

type DB struct {
	svc            *svc.ServiceContext
	c              chains.Chain
	outputParser   outputparser.Structured
	analysisChains *sqlguard.Chain
	policy         sqlguard.Policy
	visual         chains.Chain
	callbacks      callbacks.Handler
}

func NewDB(svc *svc.ServiceContext) *DB {
	return &DB{
		svc:          svc,
		outputParser: _outputParser,
		c: chains.NewLLMChain(svc.LLMs, prompts.PromptTemplate{
			Template:       _defaultDBQuery,
			InputVariables: []string{langchain.Input},
			TemplateFormat: prompts.TemplateFormatGoTemplate,
			PartialVariables: map[string]any{
				"formatting": _outputParser.GetFormatInstructions(),
			},
		}),
		visual: visual.NewVisualChains(svc.LLMs, svc.Config.Upload.SavePath),
		policy: sqlguard.DefaultPolicy(),
	}
}

func (d *DB) Name() string {
	return "db"
}

func (d *DB) Description() string {
	return `use for data analysis when no file is specified. supported db: budget`
}

func (d *DB) Chains() chains.Chain {
	return chains.NewTransform(d.transform, nil, nil)
}

func (d *DB) transform(ctx context.Context, inputs map[string]any, options ...chains.ChainCallOption) (map[string]any, error) {
	fmt.Println("analysis --- db --- start")
	if err := RequireAnalysisAccess(ctx, d.svc); err != nil {
		return nil, err
	}

	if err := d.initDB(); err != nil {
		return nil, err
	}
	if err := d.applyPolicy(ctx); err != nil {
		return nil, err
	}

	rawInput := inputs[langchain.Input].(string)
	safeInput := defend.Sanitize(rawInput, defend.DefaultSanitizeConfig())
	inputs[langchain.Input] = safeInput
	ctx = d.withTenant(ctx)

	out, err := chains.Predict(ctx, d.c, inputs, options...)
	if err != nil {
		return nil, err
	}
	fmt.Printf("analysis db chains.Predict out %v\n", out)

	query, err := d.extractQuery(ctx, out)
	if err != nil {
		if _, ok := router.AsNotHandles(err); ok {
			return nil, err
		}
		return nil, missingDB(map[string]string{"input": safeInput})
	}
	fmt.Printf("analysis db d.extractQuery query %v\n", query)

	prep, err := d.analysisChains.Prepare(ctx, query, options...)
	if err != nil {
		if errors.Is(err, sqlguard.ErrInvalidIR) || errors.Is(err, sqlguard.ErrUnknownTable) {
			return nil, missingDB(queryPartial(query, safeInput))
		}
		return nil, err
	}

	if d.svc == nil || d.svc.SQLDrafts == nil {
		return nil, errors.New("sql draft store is not configured")
	}

	uid := token.GetUId(ctx)
	if uid == "" {
		return nil, errors.New("请先登录后再执行数据分析")
	}

	draft := d.svc.SQLDrafts.Put(sqlguard.Draft{
		ID:         ksuid.New().String(),
		UserID:     uid,
		SQL:        prep.SQL,
		Query:      prep.Query,
		Tables:     prep.Tables,
		IR:         prep.IR,
		RelationID: relationID(inputs),
	})
	audit.Event(ctx, "sql.draft", "uid", uid, "draftId", draft.ID, "tables", strings.Join(prep.Tables, ","))
	return marshalSQLDraft(d.showSQL(ctx), draft, prep.SQL, prep.Query)
}

func marshalSQLDraft(showSQL bool, draft sqlguard.Draft, sql, userQuery string) (map[string]any, error) {
	resp, err := sqlDraftChatResp(showSQL, draft)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(resp)
	if err != nil {
		return nil, err
	}
	return map[string]any{langchain.OutPut: string(body)}, nil
}

func sqlDraftChatResp(showSQL bool, draft sqlguard.Draft) (*domain.ChatResp, error) {
	var (
		summary string
		tables  []string
		columns []string
		err     error
	)
	if !draft.IR.Empty() {
		summary, tables, columns = sqlguard.ExplainIR(draft.IR, sqlguard.DefaultTopK)
	} else {
		summary, tables, columns, err = sqlguard.Explain(draft.SQL, draft.Query, sqlguard.DefaultTopK)
		if err != nil {
			return nil, err
		}
	}
	limit := sqlguard.DefaultTopK
	if draft.IR.Limit > 0 {
		limit = draft.IR.Limit
	}
	data := domain.SQLDraft{
		DraftId: draft.ID,
		Summary: summary,
		Tables:  tables,
		Columns: columns,
		Limit:   limit,
	}
	if showSQL {
		data.SQL = draft.SQL
	}
	return &domain.ChatResp{
		ChatType: int(domain.ConfirmSQL),
		Data:     data,
	}, nil
}

func relationID(inputs map[string]any) int {
	switch v := inputs["relationId"].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

// ExecuteConfirmed runs the stored SELECT, answers in Chinese, and optionally draws a chart.
func (d *DB) ExecuteConfirmed(ctx context.Context, draft sqlguard.Draft, options ...chains.ChainCallOption) (*domain.ChatResp, error) {
	if err := RequireAnalysisAccess(ctx, d.svc); err != nil {
		return nil, err
	}
	if err := d.initDB(); err != nil {
		return nil, err
	}
	if err := d.applyPolicy(ctx); err != nil {
		return nil, err
	}
	ctx = d.withTenant(ctx)
	audit.Event(ctx, "sql.execute", "uid", token.GetUId(ctx), "draftId", draft.ID)
	res, err := d.analysisChains.Execute(ctx, &sqlguard.Prepared{
		SQL:    draft.SQL,
		Query:  draft.Query,
		Tables: draft.Tables,
		IR:     draft.IR,
	}, options...)
	if err != nil {
		return nil, err
	}
	answer, _ := res["result"].(string)
	answer = defend.RedactSecrets(answer)
	return d.wrapVisual(ctx, answer, draft.Query, options...)
}

// ReviseDraft regenerates SQL from stored intent plus user feedback. Client SQL is ignored.
func (d *DB) ReviseDraft(ctx context.Context, draft sqlguard.Draft, feedback string, options ...chains.ChainCallOption) (*domain.ChatResp, error) {
	if err := RequireAnalysisAccess(ctx, d.svc); err != nil {
		return nil, err
	}
	if err := d.initDB(); err != nil {
		return nil, err
	}
	if err := d.applyPolicy(ctx); err != nil {
		return nil, err
	}
	if d.svc == nil || d.svc.SQLDrafts == nil {
		return nil, errors.New("sql draft store is not configured")
	}
	ctx = d.withTenant(ctx)
	prep, err := d.analysisChains.Revise(ctx, &sqlguard.Prepared{
		SQL:    draft.SQL,
		Query:  draft.Query,
		Tables: draft.Tables,
		IR:     draft.IR,
	}, feedback, options...)
	if err != nil {
		return nil, err
	}
	next := d.svc.SQLDrafts.Put(sqlguard.Draft{
		ID:         ksuid.New().String(),
		UserID:     draft.UserID,
		SQL:        prep.SQL,
		Query:      prep.Query,
		Tables:     prep.Tables,
		IR:         prep.IR,
		RelationID: draft.RelationID,
	})
	return sqlDraftChatResp(d.showSQL(ctx), next)
}

func (d *DB) wrapVisual(ctx context.Context, answer, userQuery string, options ...chains.ChainCallOption) (*domain.ChatResp, error) {
	v, err := chains.Call(ctx, d.visual, map[string]any{
		langchain.Input: visualInput(userQuery, answer),
	}, options...)
	if err != nil {
		fmt.Println("visual : ", err)
		return &domain.ChatResp{ChatType: 0, Data: answer}, nil
	}
	imgUrl, ok := v[visual.ImgUrl].(string)
	if !ok || imgUrl == "" {
		return &domain.ChatResp{ChatType: 0, Data: answer}, nil
	}
	return &domain.ChatResp{
		ChatType: domain.ImgAndText,
		Data: domain.ImgAndTextResp{
			Text: answer,
			Url:  httpx.JoinHostPath(d.svc.Config.Upload.Host, imgUrl),
		},
	}, nil
}

func (d *DB) extractQuery(ctx context.Context, out string) (map[string]any, error) {
	res, err := d.outputParser.Parse(out)
	if err != nil {
		return nil, err
	}

	data, ok := res.(map[string]string)
	if !ok {
		return nil, missingDB(map[string]string{})
	}
	dbName := strings.TrimSpace(data[_destinations])
	rewritten := strings.TrimSpace(data[langchain.Input])
	partial := map[string]string{
		"destinations": dbName,
		"input":        rewritten,
	}
	if dbName == "" || dbName == router.Empty || !d.policy.AllowTable(dbName) {
		return nil, missingDB(partial)
	}
	if rewritten == "" {
		return nil, missingDB(partial)
	}

	return map[string]any{
		"query":              rewritten,
		"table_names_to_use": []string{dbName},
	}, nil
}

func visualInput(userQuery, answer string) string {
	q := strings.TrimSpace(userQuery)
	a := strings.TrimSpace(answer)
	if q == "" {
		return a
	}
	if a == "" {
		return q
	}
	return "User request:\n" + q + "\n\nAnswer:\n" + a
}

func (d *DB) initDB() (err error) {
	if d.analysisChains != nil {
		return nil
	}

	fmt.Println("analysis ---initDB")

	dsn := strings.TrimSpace(d.svc.Config.MysqlReadDns)
	if dsn == "" {
		dsn = d.svc.Config.MysqlDns
	}
	engine, err := newReadOnlyMySQL(dsn)
	if err != nil {
		return err
	}

	db, err := sqldatabase.NewSQLDatabase(engine, map[string]struct{}{
		"admin": {},
	})
	if err != nil {
		return err
	}

	d.analysisChains = sqlguard.NewChain(d.svc.LLMs, sqlguard.DefaultTopK, db, d.policy)
	return nil
}

func (d *DB) applyPolicy(ctx context.Context) error {
	p := sqlguard.DefaultPolicy()
	if d.svc != nil && d.svc.Whitelist != nil {
		loaded, err := d.svc.Whitelist.SQLPolicy(ctx)
		if err != nil {
			return err
		}
		p = loaded
	}
	d.policy = p
	if d.analysisChains != nil {
		d.analysisChains.SetPolicy(p)
	}
	return nil
}

func (d *DB) withTenant(ctx context.Context) context.Context {
	return sqlguard.WithTenant(ctx, d.loadTenant(ctx))
}

func (d *DB) showSQL(ctx context.Context) bool {
	return d.loadTenant(ctx).IsSystem
}

func (d *DB) loadTenant(ctx context.Context) sqlguard.Tenant {
	uid := token.GetUId(ctx)
	t := sqlguard.Tenant{UserID: uid}
	if d.svc == nil || uid == "" {
		return t
	}
	user, err := d.svc.UserModel.FindOne(ctx, uid)
	if err == nil && user != nil {
		t.UserName = user.Name
		t.IsSystem = user.IsSystem
	}
	if dep, err := d.svc.DepartmentUserModel.FindByUserId(ctx, uid); err == nil && dep != nil && dep.DepId != "" {
		t.DeptIDs = []string{dep.DepId}
	}
	return t
}
