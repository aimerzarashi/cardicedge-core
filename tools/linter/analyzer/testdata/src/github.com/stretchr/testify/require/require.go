package require

// Stub package for analyzer testing.
// This is not the real testify library, just minimal signatures for analysis.

type TestingT interface {
	Errorf(format string, args ...interface{})
}

func Equal(t TestingT, expected, actual interface{}, msgAndArgs ...interface{}) {
}

func NoError(t TestingT, err error, msgAndArgs ...interface{}) {
}

func Contains(t TestingT, s, contains interface{}, msgAndArgs ...interface{}) {
}
