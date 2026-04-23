package compute

// IsAnyLetter reports whether symbol is an ASCII letter in either case.
func IsAnyLetter(symbol rune) bool {
	return (symbol >= LetterRangeLower[0] &&
		symbol <= LetterRangeLower[1]) ||
		(symbol >= LetterRangeUpper[0] &&
			symbol <= LetterRangeUpper[1])
}

// IsUpperLetter reports whether symbol is an upper-case ASCII letter.
func IsUpperLetter(symbol rune) bool {
	return symbol >= LetterRangeUpper[0] && symbol <= LetterRangeUpper[1]
}

// IsDigit reports whether symbol is an ASCII decimal digit.
func IsDigit(symbol rune) bool {
	return symbol >= DigitRange[0] && symbol <= DigitRange[1]
}

// IsPunctuation reports whether symbol belongs to the allow-list in
// [Punctuation].
func IsPunctuation(symbol rune) bool {
	for _, pct := range Punctuation {
		if symbol == pct {
			return true
		}
	}
	return false
}

// ValidateCommand reports whether raw consists solely of upper-case ASCII
// letters and is therefore a syntactically valid command name.
func ValidateCommand(raw string) bool {
	for _, symbol := range raw {
		ok := IsUpperLetter(symbol)
		if !ok {
			return false
		}
	}
	return true
}

// ValidateArgument reports whether raw consists solely of letters, digits
// and characters listed in [Punctuation]. Empty input is considered valid.
func ValidateArgument(raw string) bool {
	for _, symbol := range raw {
		ok := IsAnyLetter(symbol) || IsDigit(symbol) || IsPunctuation(symbol)
		if !ok {
			return false
		}
	}
	return true
}
