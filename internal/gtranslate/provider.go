package gtranslate

import (
	"trans/internal/translation"
)

type Provider struct{}

func (Provider) Name() string { return "gtranslate" }

// New builds the service. It needs no key, so there is nothing it can be
// missing; the endpoint can still be pointed elsewhere by a setting.
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
