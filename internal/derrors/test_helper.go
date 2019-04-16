package derrors

// ErrorType represents errors supported by the derrors package. It should
// only be used for test assertions.
type ErrorType uint32

// Enumerate error types in the derrors package.
const (
	NilErrorType ErrorType = iota
	NotFoundType
	InvalidArgumentType
	UncategorizedErrorType
)

func (t ErrorType) String() string {
	return [...]string{"nil", "NotFound", "InvalidArgument", "Uncategorized"}[t]
}

// Type categorizes the given error based on the derrors error semantics. It
// should only be used for test assertions. The Is<error> helper methods should
// be used for non-test code.
func Type(err error) ErrorType {
	if err == nil {
		return NilErrorType
	}
	switch err.(type) {
	case notFound:
		return NotFoundType
	case invalidArgument:
		return InvalidArgumentType
	default:
		return UncategorizedErrorType
	}
}
