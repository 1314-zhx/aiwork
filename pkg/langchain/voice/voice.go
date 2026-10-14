/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package voice

import (
	"context"
	"github.com/sashabaranov/go-openai"
)

type Voice struct {
	openai *openai.Client
	model  string
}

type Options struct {
	Model       string
	Language    string
	Temperature float32
}

type Option func(options *Options)

func NewVoice(client *openai.Client, opts ...Option) *Voice {
	opt := defaultOptions()
	for _, o := range opts {
		o(&opt)
	}

	return &Voice{
		openai: client,
		model:  opt.Model,
	}
}

func defaultOptions() Options {
	return Options{
		Model: "whisper-1",
	}
}

func WithModel(model string) Option {
	return func(options *Options) {
		options.Model = model
	}
}

func WithLanguage(language string) Option {
	return func(options *Options) {
		options.Language = language
	}
}

func WithTemperature(temperature float32) Option {
	return func(options *Options) {
		options.Temperature = temperature
	}
}

func (v *Voice) Transcriptions(ctx context.Context, filepath, prompt string, opts ...Option) (string, error) {
	opt := defaultOptions()
	for _, o := range opts {
		o(&opt)
	}

	req := openai.AudioRequest{
		Model:       v.model,
		FilePath:    filepath,
		Prompt:      prompt,
		Language:    opt.Language,
		Temperature: opt.Temperature,
	}

	res, err := v.openai.CreateTranscription(ctx, req)
	if err != nil {
		return "", err
	}

	return res.Text, nil
}
