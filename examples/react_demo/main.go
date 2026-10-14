package main

import (
	"fmt"
	"strings"
)

type Fres struct {
	Type   string
	Output string
}

type Tfc func(string) Fres

type PlanResult struct {
	Kind   string // "action" | "finish"
	Tool   string
	Input  string
	Answer string
}

type Step struct {
	Tool        Tfc
	Input       string
	Observation string
}

type Executor struct {
	Step
	tools map[string]Tfc
}

func ToolFind(_ string) Fres {
	return Fres{Type: "success", Output: `{"title":"写周报"}`}
}

func ToolAdd(input string) Fres {
	return Fres{Type: "error", Output: "ERROR: 需要用户确认，未写库"}
}

// LLM 模拟模型：没有 observation 就选工具；有成功结果就 finish。
func LLM(question, observation string) PlanResult {
	if strings.Contains(observation, `"title"`) {
		return PlanResult{Kind: "finish", Answer: "你有一条待办：写周报"}
	}
	if strings.Contains(question, "查") || strings.Contains(question, "gongju") {
		return PlanResult{Kind: "action", Tool: "todo_find", Input: "{}"}
	}
	return PlanResult{Kind: "finish", Answer: "我没听懂，请再说一次"}
}

func (e *Executor) Plan(question string) PlanResult {
	return LLM(question, e.Observation)
}

func ReAct(e *Executor, question string, maxIter int) string {
	e.Observation = ""
	for i := 0; i < maxIter; i++ {
		p := e.Plan(question)
		if p.Kind != "action" {
			return p.Answer
		}
		fn, ok := e.tools[p.Tool]
		if !ok {
			e.Observation = "ERROR: unknown tool " + p.Tool
			continue
		}
		fres := fn(p.Input)
		e.Observation = fres.Output
		// 成功：下一跳让模型 finish，不要在这里直接 return 工具原文
		if fres.Type != "success" {
			continue
		}
	}
	return "超过最大跳数，未 finish"
}

func main() {
	e := &Executor{
		tools: map[string]Tfc{
			"todo_find": ToolFind,
			"todo_add":  ToolAdd,
		},
	}
	fmt.Println(ReAct(e, "查一下我的待办", 2))
}
