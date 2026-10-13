package ui

func retryOptions(record taskRecord, fallback Options) Options {
	if record.Execution != nil {
		fallback = *record.Execution
	}
	fallback.Input = record.Input
	fallback.Output = record.Result
	if record.Execution != nil || record.Config != "" {
		fallback.Config = record.Config
	}
	return fallback
}
