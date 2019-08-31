// Package errors provides error types for wtfptd
package errors

import (
	"fmt"
)

type Op string

type WtftpdError struct {
	Err error // the underlying error if any
	Op  Op
	Msg string
}

func E(args ...interface{}) error {
	var ret WtftpdError
	for _, arg := range args {
		switch targ := arg.(type) {
		case string:
			ret.Msg = targ
		case Op:
			ret.Op = targ
		case error:
			ret.Err = targ
		default:
			panic("unexpected type passed in error")
		}
	}
	return ret
}

func (w WtftpdError) Error() string {
	var ret string
	if w.Err == nil {
		ret = fmt.Sprintf("[%s] %s", w.Op, w.Msg)
	} else {
		ret = fmt.Sprintf("[%s] %s, err:%s", w.Op, w.Msg, w.Err)
	}
	return ret
}
