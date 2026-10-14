/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package chatinternal

import (
	"context"
	"encoding/json"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/memory"
	"github.com/tmc/langchaingo/schema"
	"aiworkc/internal/domain"
	"aiworkc/pkg/langchain"
	"aiworkc/pkg/langchain/memoryx"
	"aiworkc/pkg/langchain/visual"
	"testing"
	"time"
)

func TestNewAnalysisHandle_DB(t *testing.T) {
	t.Log(svcTest.Config.MysqlDns)

	memory := memory.NewSimple()
	a := NewAnalysisHandle(svcTest, memory)
	ctx := context.Background()

	res, err := chains.Call(ctx, a.Chains(), map[string]any{
		"input": "分析财务预算中待发生、进行中、已完成的各项金额",
	})

	t.Log(res, " ", err)
}

func TestNewAnalysisHandle_File(t *testing.T) {
	t.Log(svcTest.Config.MysqlDns)

	memory := memoryx.NewMemoryx(func() schema.Memory {
		m := memoryx.NewSummaryBuffer(svcTest.LLMs, 50, memoryx.WithCallback(svcTest.Callbacks),
			memoryx.WithModel("qwen-turbo"))

		m.InputKey = langchain.Input
		return m
	})

	ctx := context.WithValue(context.Background(), langchain.ChatId, "999")
	testUploadFile(ctx, memory, t)

	// 输出
	t.Log(memory.LoadMemoryVariables(ctx, map[string]any{}))

	a := NewAnalysisHandle(svcTest, memory)
	res, err := chains.Call(ctx, a.Chains(), map[string]any{
		"input": "请根据上传的财务预算报表分析财务预算中待发生、进行中、已完成的各项金额",
	})

	t.Log(res, " ", err)
}

func TestVisualBarChart(t *testing.T) {
	t.Log("测试可视化柱状图生成")

	// 直接调用 BarTool 生成柱状图，跳过可视化路由链
	barTool := visual.NewBarTool(svcTest.LLMs, svcTest.Config.Upload.SavePath)
	ctx := context.Background()

	// 传入图表配置和分析数据
	inputs := map[string]string{
		visual.Title: "预算金额统计",
		visual.X:      "项目",
		visual.Y:      "金额（元）",
		langchain.Input: `{"员工每日餐补": 50, "北京办公室房租": 15000, "阿里云服务器年费": 28000, "全员年度体检": 80000}`,
	}

	res, err := barTool.Call(ctx, inputs)

	t.Log("可视化结果:", res)
	if err != nil {
		t.Fatalf("可视化失败: %v", err)
	}

	imgUrl, ok := res[visual.ImgUrl].(string)
	if !ok || imgUrl == "" {
		t.Fatal("未生成图片URL")
	}

	t.Logf("柱状图已生成: %s", imgUrl)
}

func testUploadFile(ctx context.Context, memory schema.Memory, t *testing.T) {
	b, err := json.Marshal([]*domain.ChatFile{
		{
			Path: svcTest.Config.Upload.SavePath + "1.xlsx",
			Name: "财务预算报表",
			Time: time.Now(),
		},
	})

	if err != nil {
		t.Fatal(err)
	}

	err = memory.SaveContext(ctx, map[string]any{
		langchain.Input: string(b),
	}, map[string]any{
		langchain.Input: "upload file",
	})

	if err != nil {
		t.Fatal(err)
	}
}
