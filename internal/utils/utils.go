package utils

func MustReturn[T any](normal T, err error) T {
	if err != nil {
		panic(err.Error())
	}
	return normal
}

func Must(err error) {
	if err != nil {
		panic(err.Error())
	}
}

// IsChannelOpen checks if the channel is open
func IsChannelOpen[T any](ch chan T) bool {
	if len(ch) != 0 {
		return true
	}
	// Check if the channel is still open
	select {
	case _, ok := <-ch:
		if !ok {
			return false
		}
	default:
	}
	return true
}

func PanicIf(cond bool, msg string) {
	if cond {
		panic(msg)
	}
}

func PanicIfErr(err error) {
	if err != nil {
		panic(err.Error())
	}
}
