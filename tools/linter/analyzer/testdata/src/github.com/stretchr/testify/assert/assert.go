package assert

// Stub package for analyzer testing.
// This is not the real testify library, just minimal signatures for analysis.

type TestingT interface {
	Errorf(format string, args ...interface{})
}

func Equal(t TestingT, expected, actual interface{}, msgAndArgs ...interface{}) bool {
	return true
}

func NoError(t TestingT, err error, msgAndArgs ...interface{}) bool {
	return true
}

func True(t TestingT, value bool, msgAndArgs ...interface{}) bool {
	return true
}

func Nil(t TestingT, object interface{}, msgAndArgs ...interface{}) bool {
	return true
}

func Contains(t TestingT, s, contains interface{}, msgAndArgs ...interface{}) bool {
	return true
}
