package service

import (
	"context"

	"trans/internal/promptflow"
	"trans/internal/translation"
)

// usageReporter says what a service has spent in the words the popup asks in.
type usageReporter struct{ reporter translation.UsageReporter }

func (u usageReporter) Usage(ctx context.Context) (promptflow.Usage, error) {
	spent, err := u.reporter.Usage(ctx)
	if err != nil {
		return promptflow.Usage{}, err
	}
	return promptflow.Usage{Used: spent.Used, Limit: spent.Limit}, nil
}

// UsageReporter asks a service what it has spent, if it keeps count at all. It
// answers false for the services that do not, which is how the popup ends up
// showing no count rather than a zero. Only here do both vocabularies meet, so
// the packages that own them need not know of each other.
func UsageReporter(translator translation.Translator) (promptflow.UsageReporter, bool) {
	reporter, err := translation.ReporterOf(translator)
	if err != nil {
		return nil, false
	}
	return usageReporter{reporter}, true
}
