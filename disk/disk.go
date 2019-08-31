// Package disk is a package providing functionality for
// a file store to wtftpd.
package disk

import (
	"context"
	"fmt"
	"sync"
	"wtftpd/errors"
	"wtftpd/log"
)

// message request types
const (
	reqRead = iota
	reqWrite
	repRead
	repWrite
)

// MapDisk is a map backed implementation of the Disk interface
// it's request satisfy the io.Reader/io.Writer interfaces in order
// to get/put bytes on it. It requires a string filename in order to
// associate them on them map. It is synchronized over channels.
type MapDisk struct {
	store  map[string][]byte
	inChan chan Request
}

// NewMapDisk creates a new MapDisk instance and launches a goroutine
// to handle it. It needs a context for cancellation and a waitgroup.
// It adds itself to the waitgroup before launcing so the user shouldn't
// add it.
func NewMapDisk(ctx context.Context, wg *sync.WaitGroup) *MapDisk {
	md := &MapDisk{
		store:  make(map[string][]byte),
		inChan: make(chan Request),
	}
	wg.Add(1)
	go md.serve(ctx, wg)
	return md
}

func (m *MapDisk) serve(ctx context.Context, wg *sync.WaitGroup) {
	const op errors.Op = "MapDisk.serve"
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			log.DiskInfof("terminating from context")
			return
		case req := <-m.inChan:
			log.DiskTracef("received :%v+", req)
			switch req.reqType {
			case reqRead:
				if data, ok := m.store[req.fname]; ok {
					sendReply(wg, req.resChan, true, data, nil)
				} else {
					sendReply(wg, req.resChan, true, nil, mkNXDiskError(op, req.fname))
				}
			case reqWrite:
				if _, ok := m.store[req.fname]; ok {
					log.DiskDebugf("replacing data in filename :%s", req.fname)
				} else {
					log.DiskTracef("creating new file:%s", req.fname)
				}
				m.store[req.fname] = req.data
				sendReply(wg, req.resChan, false, nil, nil)
			default:
				log.DiskFatalf("unknown req type in the disk subsystem:%v+", req)
			}
		}
	}
}

func sendReply(wg *sync.WaitGroup, to chan<- reply, isRead bool, data []byte, err error) {
	var rep reply
	if isRead {
		rep = newReadReply(data, err)
	} else {
		rep = newWriteReply(err)
	}
	go func() {
		to <- rep
	}()

}

// Request is a message type for the disk subsystem
// it wraps a channel to which a response can be delivered
type Request struct {
	reqType int
	inChan  chan Request
	resChan chan reply
	fname   string
	data    []byte
}

// Request implements the io.Reader interface if it's a read request
// it will error if the buffer is too small.
func (r Request) Read(to []byte) (int, error) {
	const op errors.Op = "Request.Read"
	if r.reqType != reqRead {
		return 0, mkReqDiskError(op, "Read called on a write request")
	}
	if len(to) == 0 {
		return 0, nil // no buffer
	}
	r.inChan <- r
	rep := <-r.resChan
	if rep.err != nil {
		return 0, rep.err
	}
	if len(to) < len(rep.data) {
		return 0, mkReqDiskError(op, "destination buffer too small")
	}
	hm := copy(to, rep.data)
	return hm, nil
}

func (r Request) Write(from []byte) (int, error) {
	const op errors.Op = "Request.Write"
	if r.reqType != reqWrite {
		return 0, mkReqDiskError(op, "Write called on a read request")
	}
	r.data = make([]byte, len(from))
	copy(r.data, from)
	r.inChan <- r
	rep := <-r.resChan
	return len(from), rep.err
}

// NewReadRequest creates a new request
func (m *MapDisk) NewReadRequest(fname string) Request {
	return Request{
		reqType: reqRead,
		inChan:  m.inChan,
		fname:   fname,
		resChan: make(chan reply),
	}
}

// NewWriteRequest creates a new write request
func (m *MapDisk) NewWriteRequest(fname string) Request {
	return Request{
		reqType: reqWrite,
		inChan:  m.inChan,
		fname:   fname,
		resChan: make(chan reply),
	}
}

// reply is a disk susbsystem reply type
type reply struct {
	repType int
	data    []byte
	err     error
}

// newReadReply constructs a disk read reply with the specified data and possible error
func newReadReply(data []byte, err error) reply {
	return reply{
		repType: repRead,
		data:    data,
		err:     err,
	}
}

// newWriteReply constructs a disk write reply with a possible error
func newWriteReply(err error) reply {
	return reply{
		repType: repWrite,
		err:     err,
	}
}

func mkNXDiskError(op errors.Op, a string) error {
	return errors.E(op, fmt.Sprintf("non existant filename:%s", a))
}

func mkReqDiskError(op errors.Op, a string) error {
	return errors.E(op, fmt.Sprintf("request failed:%s", a))
}
