// Package errors provides error types for wtfptd
package errors

import (
	"fmt"
	"strings"
)

// Op is an operation that can error. typically named as the function that the program errors in.
type Op string

// Kind is a categorization of the error, in orde to be able to decide on it. For example
// a client error might be communicated back to the client
type Kind uint8

const (
	// ClientRelated means that it might be something the client can fix
	ClientRelated Kind = iota + 1
	// TimeoutRelated could force a retry.
	TimeoutRelated
	// SequenceRelated means mixing up of the state machine
	SequenceRelated
	// NetRelated can be low level errors in the interfaces or similar
	NetRelated
	// DiskRelated is an error on the disk subsystem
	DiskRelated
	maxKind
)

var kindNames = [maxKind]string{
	"notset",
	"Client",
	"Timeout",
	"Sequence",
	"Net",
	"Disk",
}

// WtftpdError is the main wrapper error type for wtftpd. It can possibly contain an internal
// error Err, an Operation called Op which should be the name of the function that
// errored, a Kind of the error,  and provided message.
type WtftpdError struct {
	Err error
	Op  Op
	Msg string
	K   Kind
}

// Is checks a general error for being a WtftpdError type
// of a certain Kind, and if yes, it returs the underlying
// type in order for the caller to have access to the fields.
func Is(e error, k Kind) (bool, *WtftpdError) {
	switch te := e.(type) {
	case *WtftpdError:
		if te.K == k {
			return true, te
		}
	}
	return false, nil
}

// E constructs an error based on type switching on the arguments
// that it is provided. Due to it's nature, if the caller provides
// two consecutive arguments of the same time, the second one is registered.
// It can panic if it receives an unexpected type.
func E(args ...interface{}) error {
	ret := &WtftpdError{}
	for _, arg := range args {
		switch targ := arg.(type) {
		case string:
			ret.Msg = targ
		case Op:
			ret.Op = targ
		case error:
			ret.Err = targ
		case Kind:
			ret.K = targ
		default:
			panic("unexpected type passed in error")
		}
	}
	return ret
}

// Error is an implementation of the Error interface.
func (w *WtftpdError) Error() string {
	var ret strings.Builder
	fmt.Fprintf(&ret, "[")
	if w.Op != "" {
		fmt.Fprintf(&ret, "%s", w.Op)
	}
	fmt.Fprintf(&ret, "/")
	if w.K != 0 {
		fmt.Fprintf(&ret, "%s", kindNames[w.K])
	}
	fmt.Fprintf(&ret, "]")
	if w.Msg != "" {
		fmt.Fprintf(&ret, " Msg: %s", w.Msg)
	}
	if w.Err != nil {
		fmt.Fprintf(&ret, " err:%s", w.Err)
	}
	return ret.String()
}
