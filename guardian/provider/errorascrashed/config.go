package errorascrashed

import "github.com/mrasu/spec-guardian/guardian/provider"

type providerConfig struct {
	Fields []fieldRequest `json:"-"`
}

type fieldRequest struct {
	Field  string
	Output provider.OutputSpec
}
