package errors

type Operation string

type WtftpdError struct {
	Err error // the underlying error if any
	Op  Operation
}
