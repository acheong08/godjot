package djot_tokenizer

import (
	"bytes"

	"github.com/sivukhin/godjot/tokenizer"
)

var (
	NotSpaceByteMask      = tokenizer.SpaceNewLineByteMask.Negate()
	NotBracketByteMask    = tokenizer.NewByteMask([]byte("]")).Negate()
	ThematicBreakByteMask = tokenizer.NewByteMask([]byte(" \t\n*-"))

	DigitByteMask      = tokenizer.NewByteMask([]byte("0123456789"))
	LowerAlphaByteMask = tokenizer.NewByteMask([]byte("abcdefghijklmnopqrstuvwxyz"))
	UpperAlphaByteMask = tokenizer.NewByteMask([]byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ"))
	AttributeTokenMask = tokenizer.Union(
		DigitByteMask,
		LowerAlphaByteMask,
		UpperAlphaByteMask,
		tokenizer.NewByteMask([]byte(`-_:`)),
	)
)

func MatchBlockToken(
	r tokenizer.TextReader,
	initialState tokenizer.ReaderState,
	tokenType DjotToken,
) (tokenizer.Token[DjotToken], tokenizer.ReaderState, bool) {
	fail := func() (tokenizer.Token[DjotToken], tokenizer.ReaderState, bool) {
		return tokenizer.Token[DjotToken]{}, initialState, false
	}
	success := func(token tokenizer.Token[DjotToken], state tokenizer.ReaderState) (tokenizer.Token[DjotToken], tokenizer.ReaderState, bool) {
		return token, state, true
	}

	var ok bool

	initialState, ok = r.MaskRepeat(initialState, tokenizer.SpaceByteMask, 0)
	tokenizer.Assertf(ok, "MaskRepeat must match because minCount is zero")

	next := initialState // initialize next variable to initialState in order to use next, ok = r.Matcher(next, ...) structure whenever it's possible

	switch tokenType {
	case HeadingBlock:
		if next, ok = r.ByteRepeat(next, '#', 1); !ok {
			return fail()
		}
		if next, ok = r.Mask(next, tokenizer.SpaceByteMask); !ok {
			return fail()
		}
		return success(tokenizer.Token[DjotToken]{Type: tokenType, Start: initialState, End: next}, next)
	case QuoteBlock:
		if next, ok = r.Token(next, ">"); !ok {
			return fail()
		}
		if next, ok = r.Mask(next, tokenizer.SpaceNewLineByteMask); !ok {
			return fail()
		}
		return success(tokenizer.Token[DjotToken]{Type: tokenType, Start: initialState, End: next}, next)
	case DivBlock, CodeBlock:
		var symbol byte
		var attributeKey string
		switch tokenType {
		case DivBlock:
			symbol, attributeKey = ':', DivClassKey
		case CodeBlock:
			symbol, attributeKey = '`', CodeLangKey
		}

		if next, ok = r.ByteRepeat(next, symbol, 3); !ok {
			return fail()
		}

		next, ok = r.MaskRepeat(next, tokenizer.SpaceByteMask, 0)
		tokenizer.Assertf(ok, "MaskRepeat must match because minCount is zero")
		if r.IsEmptyOrWhiteSpace(next) {
			return success(tokenizer.Token[DjotToken]{Type: tokenType, Start: initialState, End: next}, next)
		}

		metaStart := next
		next, ok = r.MaskRepeat(next, NotSpaceByteMask, 1)
		tokenizer.Assertf(ok, "MaskRepeat must match because !r.IsEmpty(next) and next symbol is not in SpaceByteMask")
		metaEnd := next

		if next, ok = r.EmptyOrWhiteSpace(next); !ok {
			return fail()
		}

		token := tokenizer.Token[DjotToken]{
			Type:       tokenType,
			Start:      initialState,
			End:        next,
			Attributes: (&tokenizer.Attributes{}).Set(attributeKey, r.Select(metaStart, metaEnd)),
		}
		return success(token, next)
	case ReferenceDefBlock, FootnoteDefBlock:
		var blockToken string
		switch tokenType {
		case ReferenceDefBlock:
			blockToken = "["
		case FootnoteDefBlock:
			blockToken = "[^"
		}
		if next, ok = r.Token(next, blockToken); !ok {
			return fail()
		}
		startKey := next
		next, ok = r.MaskRepeat(next, NotBracketByteMask, 0)
		tokenizer.Assertf(ok, "MaskRepeat must match because minCount is zero")
		endKey := next

		if next, ok = r.Token(next, "]:"); !ok {
			return fail()
		}
		token := tokenizer.Token[DjotToken]{
			Type:       tokenType,
			Start:      initialState,
			End:        next,
			Attributes: (&tokenizer.Attributes{}).Set(ReferenceKey, r.Select(startKey, endKey)),
		}
		return success(token, next)
	case ThematicBreakToken:
		next, ok = r.MaskRepeat(next, ThematicBreakByteMask, 0)
		tokenizer.Assertf(ok, "MaskRepeat must match because minCount is zero")
		if !r.IsEmpty(next) {
			return fail()
		}
		// three or more * or - characters required for thematic break
		if bytes.Count(r[initialState:next], []byte{'*'}) < 3 && bytes.Count(r[initialState:next], []byte{'-'}) < 3 {
			return fail()
		}
		return success(tokenizer.Token[DjotToken]{Type: tokenType, Start: initialState, End: next}, next)
	case ListItemBlock:
		for _, simpleToken := range [...]string{"+ ", "* ", "- ", ": ", "- [ ] ", "- [x] ", "- [X] "} {
			if simple, ok := r.Token(next, simpleToken); ok {
				return success(tokenizer.Token[DjotToken]{Type: tokenType, Start: initialState, End: simple}, simple)
			}
		}

		for _, complexTokenMask := range [...]tokenizer.ByteMask{DigitByteMask, LowerAlphaByteMask, UpperAlphaByteMask} {
			// consider three valid cases: (MASK) | MASK) | MASK.
			complexNext := next
			parenOpen, parenOk := r.Token(next, "(")
			if parenOk {
				complexNext = parenOpen
			}
			if complexNext, ok = r.MaskRepeat(complexNext, complexTokenMask, 1); !ok {
				continue
			}
			if ending, ok := r.Token(complexNext, ") "); ok {
				return success(tokenizer.Token[DjotToken]{Type: tokenType, Start: initialState, End: ending}, ending)
			} else if ending, ok := r.Token(complexNext, ". "); ok && !parenOk {
				return success(tokenizer.Token[DjotToken]{Type: tokenType, Start: initialState, End: ending}, ending)
			}
		}
		return fail()
	case PipeTableBlock:
		// todo (sivukhin, 2023-10-28): check that line ends with another pipe
		if next, ok = r.Token(next, "|"); !ok {
			return fail()
		}
		return success(tokenizer.Token[DjotToken]{Type: tokenType, Start: initialState, End: next}, next)
	case ParagraphBlock:
		if r.IsEmpty(next) {
			return fail()
		}
		return success(tokenizer.Token[DjotToken]{Type: tokenType, Start: initialState, End: next}, next)
	default:
		tokenizer.Assertf(false, "unexpected djot block token type: %v", tokenType)
		return fail()
	}
}
