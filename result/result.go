package result

// Result can store either an error or a value - if Err is nil, Value may be
// read - otherwise, if Err is non-nil, Value should be considered invalid
type Result[T any] struct {
	err   error
	value T
}

// Construct a result indicating success
func OK[T any](value T) Result[T] {
	return Result[T]{
		value: value,
	}
}

// Construct a result indicating error
func Err[T any](err error) Result[T] {
	return Result[T]{
		err: err,
	}
}

func (result Result[T]) Unwrap() (T, error) {
	return result.value, result.err
}
