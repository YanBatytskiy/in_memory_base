package compute

// Command names recognised by the parser.
const (
	CommandSet = "SET"
	CommandGet = "GET"
	CommandDel = "DEL"
)

// Command identifiers stored in the WAL. Numeric form keeps the binary log
// format independent from the spelling of command names.
const (
	CommandSetID = 1
	CommandDelID = 2
)

// Allowed character ranges and expected argument counts for each command.
var (
	// Punctuation lists the non-alphanumeric characters accepted inside an
	// argument token (everything else is rejected by [ValidateArgument]).
	Punctuation = []rune{'*', '/', '_', '.'}

	// LetterRangeLower is the inclusive range of lower-case ASCII letters.
	LetterRangeLower = [2]rune{'a', 'z'}

	// LetterRangeUpper is the inclusive range of upper-case ASCII letters.
	LetterRangeUpper = [2]rune{'A', 'Z'}

	// DigitRange is the inclusive range of ASCII decimal digits.
	DigitRange = [2]rune{'0', '9'}

	// CommandSetQ is the number of arguments expected after SET (key, value).
	CommandSetQ = 2
	// CommandGetQ is the number of arguments expected after GET (key).
	CommandGetQ = 1
	// CommandDelQ is the number of arguments expected after DEL (key).
	CommandDelQ = 1
)
