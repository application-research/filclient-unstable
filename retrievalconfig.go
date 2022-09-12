package filclient

import (
	"github.com/ipld/go-ipld-prime"
	selectorparse "github.com/ipld/go-ipld-prime/traversal/selector/parse"
)

type RetrievalConfig struct {
	selector ipld.Node
}

func (cfg *RetrievalConfig) Clean() {
	if cfg.selector == nil {
		cfg.selector = selectorparse.CommonSelector_ExploreAllRecursively

	}
}

type RetrievalOption func(*RetrievalConfig)

// Sets the retrieval selector
func RetrievalWithSelector(selector ipld.Node) RetrievalOption {
	return func(cfg *RetrievalConfig) {
		cfg.selector = selector
	}
}
