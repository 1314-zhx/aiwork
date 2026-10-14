/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package chatinternal

import (
	"aiworkc/pkg/langchain"
	"aiworkc/pkg/token"
	"context"
	"fmt"
	"testing"

	"github.com/tmc/langchaingo/chains"
)

func TestApproval(t *testing.T) {
	tokenStr := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE3ODk0NjE2MDcsImlhdCI6MTc4MDgyMTYwNywiaW1vb2MuY29tIjoiNmExZmY5MTI3YTg2ZWJlNmMzNWYwZjdiIn0._z1HyfKYYSqWRH_2ncM0iH36nFZDmdJza-x0DzXvWvI"
	ctx := context.Background()
	ctx = context.WithValue(ctx, token.Authorization, tokenStr)

	chat := NewApprovalHandle(svcTest)
	res, err := chat.baseChat.transform(ctx, map[string]any{
		langchain.Input: "帮我请个病假，明天下午6点的",
		"history":       "",
	}, chains.WithCallback(svcTest.Callbacks))

	if err != nil {
		fmt.Printf("Error: %+v\n", err)
		if u, ok := err.(interface{ Unwrap() error }); ok && u.Unwrap() != nil {
			fmt.Printf("Root cause: %v\n", u.Unwrap())
		}
		t.Fatal(err)
	}

	t.Log(res)
}
