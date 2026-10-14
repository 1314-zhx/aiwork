/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package voice

import (
	"context"
	openaiSdk "github.com/sashabaranov/go-openai"
	"testing"
)

func TestVoice_Transcriptions(t *testing.T) {
	cfg := openaiSdk.DefaultConfig("")
	cfg.BaseURL = "https://api.aiproxy.io/v1"
	client := openaiSdk.NewClientWithConfig(cfg)

	v := NewVoice(client, WithModel("whisper-1"))

	// 使用传入文件路径和 prompt 进行测试
	// 实际使用时传入真实的音频文件路径，例如 "./test.wav"
	t.Log("测试语音转文字功能，请确保 ./1.mp3 存在")
	res, err := v.Transcriptions(context.Background(), "./1.mp3", "")
	if err != nil {
		t.Logf("转写结果: %v, 错误: %v", res, err)
		return
	}
	t.Logf("转写结果: %s", res)
}
