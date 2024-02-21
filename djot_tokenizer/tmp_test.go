package djot_tokenizer_test

import (
	"testing"
	"github.com/sivukhin/godjot/djot_tokenizer"
)

func TestTokenizer(t *testing.T) {
	djot_tokenizer.BuildInlineDjotTokens([]byte("___abc___"))
}
