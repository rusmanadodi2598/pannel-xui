package common

import (
	"fmt"

	"github.com/alireza0/x-ui/logger"
)

func NewErrorf(format string, a ...interface{}) error {
	msg := fmt.Sprintf(format, a...)
	return fmt.Errorf("%s", msg)
}

func NewError(a ...interface{}) error {
	msg := fmt.Sprintln(a...)
	return fmt.Errorf("%s", msg)
}

func Recover(msg string) interface{} {
	panicErr := recover()
	if panicErr != nil {
		if msg != "" {
			logger.Error(msg, "panic:", panicErr)
		}
	}
	return panicErr
}
