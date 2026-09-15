package mymemory

import (
	"trans/internal/translation"
)

type Provider struct{}

func (Provider) Name() string { return "mymemory" }

func (Provider) New(options translation.Options) (translation.Translator, error) {
	var clientOptions []Option
	if options.TargetLanguage != "" {
		clientOptions = append(clientOptions, WithTargetLanguage(options.TargetLanguage))
	}
	if options.Endpoint != "" {
		clientOptions = append(clientOptions, WithEndpoint(options.Endpoint))
	}
	return New(clientOptions...), nil
}
