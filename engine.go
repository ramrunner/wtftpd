package wtftpd

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
	"wtftpd/disk"
	"wtftpd/errors"
	"wtftpd/log"
	"wtftpd/packet"
)

const (
	maxRcvBuf      = 516     // msg id + blocknum + data
	maxDataPkt     = 512     // max data in pkt
	maxFsize       = 1 << 25 // 65535*512 bytes
	maxRetransmits = 3       // arbitrary times where we will retry a failed send/rcv
)

const (
	reqRead = iota
	reqWrite
)

const (
	udpTimeoutSeconds = 5
)

// Conf holds configuration vars
type Conf struct {
	ipstr string // ip of the interface to bind to
	port  uint16 // port to start the daemon on
	dir   string // dir to scan for initial files to load
}

// NewConf instantiates a new config
func NewConf(ip string, port uint16, dir string) *Conf {
	return &Conf{
		ipstr: ip,
		port:  port,
		dir:   dir,
	}
}

// Wtftpd is a hold the main thread context and a config.
type Wtftpd struct {
	mainThread *threadCtx
	conf       *Conf
}

// NewWtftpd returns a new Wtftpd that will listen on a cancellation context, register
// on a waitgroup and configure using a configuration struct
func NewWtftpd(ctx context.Context, wg *sync.WaitGroup, cf *Conf) (*Wtftpd, error) {
	disk := disk.NewMapDisk(ctx, wg)
	disk.Initialize(cf.dir)
	mthread, err := newThreadCtx(ctx, wg, disk, nil, cf.ipstr, cf.port)
	if err != nil {
		return nil, err
	}
	return &Wtftpd{
		mainThread: mthread,
		conf:       cf,
	}, nil
}

// clirequests is a request associated with a data input or sink that will be used
// as a fake client for testing.
type clirequest struct {
	request
	in  io.Reader
	out io.Writer
}

// a small client implementation mostly used for testing for now. It returns errors it encounters on an
// error channel.
func newWtftpdCli(ctx context.Context, wg *sync.WaitGroup, srv net.Addr, ec chan error, req clirequest) {
	defer wg.Done()
	defer close(ec) // that will notify our caller that we have exited
	thread, err := newThreadCtx(ctx, wg, nil, srv, "localhost", 0)
	if err != nil {
		ec <- err
		return
	}
	defer thread.conn.Close()
	buf := make([]byte, maxDataPkt) // our reader will read in this
	var curpkt packet.Packet
	if req.rtype == reqWrite {
		//make a wrq
		curpkt = &packet.WRQPacket{
			Fname: packet.NewNetASCII(req.fname),
			Mode:  "octet",
		}
	} else {
		//make a rrq
		curpkt = &packet.RRQPacket{
			Fname: packet.NewNetASCII(req.fname),
			Mode:  "octet",
		}
	}
	log.EngineDebugf("client is writing pkt of type %T:%v", curpkt, curpkt)
	thread.marshalWrite(curpkt)
	ateof := false
	for {
		nextpacket, addr, err := thread.rcvUnmarshal()
		if nextpacket == nil && err == nil { // timeout
			continue
		}
		log.EngineTracef("client read pkt of type %T:%v", nextpacket, nextpacket)
		if err != nil {
			ec <- err
			return
		}
		if ateof {
			log.EngineDebugf("client exiting succesfully")
			return
		}
		thread.other = addr // change to the other server port
		curpkt, err = genNextPacket(nextpacket)
		if err != nil {
			ec <- err
			return
		}
		if req.rtype == reqRead {
			if datpkt, ok := nextpacket.(*packet.DataPacket); ok {
				// write it out our writer
				hm, _ := req.out.Write(datpkt.Data)
				log.EngineDebugf("client wrote to the underlying writer %d bytes", hm)
			}
		} else {
			if datpkt, ok := curpkt.(*packet.DataPacket); ok {
				// read it from our reader
				hm, err := req.in.Read(buf)
				if err != nil {
					ec <- err
					return
				}
				if hm < maxDataPkt {
					ateof = true
				}
				datpkt.Data = buf[:hm]
			}
		}
		log.EngineTracef("client write pkt of type %T:%v", curpkt, curpkt)
		thread.marshalWrite(curpkt)
	}

}

//threadCtx keeps some context on running UDP goroutines
type threadCtx struct {
	ctx   context.Context // context for cancellation
	wg    *sync.WaitGroup // waitgroup for parent to wait on
	conn  *net.UDPConn    // UDP connection
	addr  *net.UDPAddr    // a way to refer to our UDP port
	other net.Addr        // the client that connected to us
	disk  *disk.MapDisk   // the connection to issue disk requests
}

// newThreadCtx creates a new thread context that is used when launching goroutines
// it tries to get the UDP port which should be provided from the conf for the main worker
// and be 0 to select an ephemeral port when launching children
func newThreadCtx(ctx context.Context, wg *sync.WaitGroup, disk *disk.MapDisk, other net.Addr, ipstr string, port uint16) (*threadCtx, error) {
	const op errors.Op = "newThreadCtx"
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", ipstr, port))
	if err != nil {
		return nil, errors.E(err, op, errors.NetRelated)
	}
	pc, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, errors.E(err, op, errors.NetRelated)
	}
	return &threadCtx{
		ctx:   ctx,
		wg:    wg,
		conn:  pc,
		addr:  addr,
		disk:  disk,
		other: other,
	}, nil
}

// every request is served on a new thread and is associated by a filename
// requests can be either read or write.
type request struct {
	fname   string
	rtype   int
	origPkt packet.Packet
	thread  *threadCtx
}

// packetToRequest determines the type of request to be created by the daemon depending
// on the initial packet, and denies non WRQ or RRQ initial packets
func packetToRequest(pkt packet.Packet) (rq *request, err error) {
	const op errors.Op = "packetToRequest"
	switch pkt.Type() {
	case packet.RRQ:
		rpkt := pkt.(*packet.RRQPacket)
		rq = &request{
			fname:   string(rpkt.Fname),
			rtype:   reqRead,
			origPkt: pkt,
		}
	case packet.WRQ:
		wpkt := pkt.(*packet.WRQPacket)
		rq = &request{
			fname:   string(wpkt.Fname),
			rtype:   reqWrite,
			origPkt: pkt,
		}
	default:
		err = errors.E(errors.SequenceRelated, op, fmt.Sprintf("not read or write initial packet %T", pkt))
	}
	return
}

// Serve is responsible for listening on the main tftp port and launching workers
// it selects nonblockingly for context cancellations, and on every iteration it
// tries to receive and unmarshal a packet (blockingly) from the main tftpd port.
// If there is an error at that stage that can be communicated back to the client it
// does so. If the packet is the beginning of a request that it can satisfy it fires
// up a new goroutine to satisfy that and goes back into listening for more requests.
func (w Wtftpd) Serve() {
	mt := w.mainThread
	defer mt.wg.Done()
	defer mt.conn.Close()

	// since readfrom blocks we launch a worker to do that while we can respond to context
	// cancellations
	for {
		select {
		case <-mt.ctx.Done():
			return
		default:
			pkt, addr, err := w.mainThread.rcvUnmarshal()
			if err != nil {
				log.EngineFileLogf("could not unmarshal pkt: %s", err)
				if ok, terr := errors.Is(err, errors.ClientRelated); ok {
					// we need to set his addr to notify him
					w.mainThread.other = addr
					epkt := &packet.ErrPacket{
						ErrCode: 1,
						Msg:     packet.NewNetASCII(terr.Msg),
					}
					w.mainThread.marshalWrite(epkt)
				}
			}
			if pkt == nil { // we're here due to timeout, go back
				continue
			}
			log.EngineTracef("[%d] received packet:%v of from :%v", mt.addr.Port, pkt.Type(), addr)
			rq, err := packetToRequest(pkt)
			if err != nil {
				log.EngineError(err)
				log.EngineFileLogf("intercepted packet: %v of type:%T, request error:%v\n", pkt, pkt, err)
				continue
			} else {
				log.EngineFileLogf("intercepted packet: %v of type:%T, request:%v\n", pkt, pkt, rq)
			}

			// going from ip->string->back to udpaddr and ip sucks but i know this ip is valid
			// since i got it from a resolveaddr. 0 will give me a system allocated portnum.
			nctx, err := newThreadCtx(mt.ctx, mt.wg, mt.disk, addr, mt.addr.IP.String(), 0)
			if err != nil {
				log.EngineError(err)
				continue
			}
			rq.thread = nctx
			rq.thread.wg.Add(1)
			go rq.serve()
		}
	}
}

// genNextPacket is the main state machine tracker. it creates a valid nextpacket
// with the correct blocknumer, and also detects when the maximum blocknumber is reached.
func genNextPacket(prev packet.Packet) (packet.Packet, error) {
	const op errors.Op = "genNextPacket"
	switch typedpkt := prev.(type) {
	case *packet.RRQPacket: // the initial request packet. give him the first block
		return &packet.DataPacket{BlockNumber: 1}, nil
	case *packet.WRQPacket: // ack him on 0
		return &packet.AckPacket{BlockNumber: 0}, nil
	case *packet.AckPacket:
		if typedpkt.BlockNumber >= 65534 { // max file size reached.
			return nil, errors.E(errors.ClientRelated, op, "reached max blocknumber")
		}
		return &packet.DataPacket{BlockNumber: typedpkt.BlockNumber + 1}, nil
	case *packet.DataPacket:
		if typedpkt.BlockNumber > 65534 { // max file size reached. that should be the last we get
			return nil, errors.E(errors.ClientRelated, op, "reached max blocknumber")
		}
		return &packet.AckPacket{BlockNumber: typedpkt.BlockNumber}, nil
	case *packet.ErrPacket:
		return nil, errors.E(errors.ClientRelated, op, string(typedpkt.Msg))
	}
	return nil, nil // notreached
}

// setNextOpDeadline sets the deadline on the net.Conn
func (t *threadCtx) setNextOpDeadline() {
	const op errors.Op = "setNextOpDeadline"
	dline := time.Now().Add(udpTimeoutSeconds * time.Second)
	if err := t.conn.SetDeadline(dline); err != nil {
		log.EngineFatal(errors.E(op, errors.NetRelated, err))
	}
	return
}

// marshalWrite marshals a packet, sets the deadline, and write it on
// the connection
func (t *threadCtx) marshalWrite(a packet.Packet) error {
	const op errors.Op = "marshalWrite"
	pbytes, err := packet.Marshal(a)
	if err != nil {
		log.EngineError(err)
		return err
	}
	t.setNextOpDeadline()
	if n, err := t.conn.WriteTo(pbytes, t.other); err != nil {
		log.EngineError(errors.E(op, errors.NetRelated, err))
	} else {
		log.EngineTracef("wrote %d bytes to %d", n, t.other)
	}
	return err

}

// rcvUnmarshal sets the deadline, creates a buffer and receives data
// on it from the conn. in the end it parses the packet.
func (t *threadCtx) rcvUnmarshal() (packet.Packet, net.Addr, error) {
	const op errors.Op = "rcvUnmarshal"
	buf := make([]byte, maxRcvBuf)
	t.setNextOpDeadline()
	n, addr, err := t.conn.ReadFrom(buf)
	if n != 0 {
		log.EngineTracef("read %d bytes from %d", n, addr)
	}
	if e, ok := err.(net.Error); ok {
		if e.Timeout() { // just return
			return nil, nil, err
		}
		log.EngineError(errors.E(op, errors.NetRelated, err))
		return nil, nil, err
	}
	pkt, err := packet.ParsePacket(buf[:n])
	if err != nil {
		log.EngineError(err)
		// here we return the addr because we might want to notify the cli
		return nil, addr, err
	}
	return pkt, addr, nil
}

// serve satisfies the requests invoked by the main thread.
// if it can straight way satisfy / or deny a read , it tries to read that
// file from the disk in memory . If it doesn't exist it errors early.
// if the file exists, or if it a write request that needs to fill a buffer
// before disk write is attempted, it goes in a loop where on every step
// it does one send and then one receive. During this it updates the curpkt
// and uses it to get the next valid packet from  genNextPacket. While doing
// that it also takes care for not accepting receives from other senders,
// blocksizes maxing out, or retransmission if ACK's were not received form the
// other end.
func (r *request) serve() {
	// the connection will be closed in the end to free any potentially blocked
	// listeners
	defer r.thread.wg.Done()
	defer r.thread.conn.Close()
	// in this far from perfect design, the following vars
	// are sadly global state to this func. Make sure not to shadow
	// them using :=
	var (
		curpkt    packet.Packet // the current packet involved in the lockstep
		curBufInd int           // this will be tracking where we are in the reads/writes
		advance   int           // this will be how much we advanced on each iteration. it will increase curBufInd
		retrans   int           // current retransmit counter
	)

	// allocate a glorious 32mb buffer
	buf := make([]byte, maxFsize)
	curpkt = r.origPkt // set our packet to the incoming one.
	log.EngineDebugf("new request thread created:%+v", r)

	if r.rtype == reqRead {
		// check to see if it's there
		dr := r.thread.disk.NewReadRequest(r.fname)
		fsize, err := dr.Read(buf)
		// truncate our file to it's actual size
		buf = buf[:fsize]
		if err != nil { // we should see if this an NX error
			epkt := &packet.ErrPacket{
				ErrCode: 1,
				Msg:     packet.NewNetASCII("this is not the file you're looking for"),
			}
			r.thread.marshalWrite(epkt)
			return
		}
		log.EngineDebugf("the disk subsystem returned a file of size:%d", fsize)
	}
	for {
		if retrans == 0 { // if we're not dealing with a retransmit , change packet state
			var nperr error
			curpkt, nperr = genNextPacket(curpkt)
			if nperr != nil {
				log.EngineError(nperr)
				if ok, te := errors.Is(nperr, errors.ClientRelated); ok { // if the client should know about max blocksize
					epkt := &packet.ErrPacket{
						ErrCode: 4,
						Msg:     packet.NewNetASCII(te.Msg),
					}
					r.thread.marshalWrite(epkt)
				}
				return
			}
		} else {
			log.EngineDebugf("%d retransmit of %v", retrans, curpkt)
		}
		advance = 0 // how much we will advance our buf index this iteration
		select {
		case <-r.thread.ctx.Done():
			log.EngineDebugf("terminating worker from context cancellation")
			return
		default:
			// if the next packet is DAT let's fill it.
			curDatBlocknum := uint16(0)
			if datpkt, ok := curpkt.(*packet.DataPacket); ok {
				curDatBlocknum = datpkt.BlockNumber
				if blen := len(buf[curBufInd:]); blen > maxDataPkt {
					advance = maxDataPkt
				} else {
					// if the buf is empty means we're done.
					if blen == 0 {
						return
					}
					// last sending packet
					advance = blen
				}
				datpkt.Data = buf[curBufInd : curBufInd+advance]

			}
			log.EngineTracef("sending packet:%T %+v", curpkt, curpkt)
			if werr := r.thread.marshalWrite(curpkt); werr != nil {
				log.EngineError(werr)
				return
			}
			rpkt, addr, rerr := r.thread.rcvUnmarshal()
			if rerr != nil { // could it be a timeout?
				if e, ok := rerr.(net.Error); ok && e.Timeout() {
					//lets retransmit the same packet
					if retrans++; retrans > maxRetransmits {
						//that's it we're giving up.
						return
					}
					continue
				}
				// something else happened. log it and quit
				log.EngineError(rerr)
				return
			}
			log.EngineTracef("received packet:%T %+v", rpkt, rpkt)
			if addr.String() != r.thread.other.String() {
				// we received something from another source than expected.
				// RFC says error our previous peer and exit.
				epkt := &packet.ErrPacket{
					ErrCode: 5,
					Msg:     packet.NewNetASCII("packet received from another source."),
				}
				r.thread.marshalWrite(epkt)
				return

			}
			switch typedpkt := rpkt.(type) {
			case *packet.AckPacket:
				// check that the block matches.
				if typedpkt.BlockNumber == curDatBlocknum {
					// great. Advance our buffer.
					curBufInd += advance
					// zero out our retransmits
					retrans = 0
				} else {
					log.EngineErrorf("ack with wrong block number received.")
					return
				}
			case *packet.DataPacket:
				if len(typedpkt.Data) == 0 {
					log.EngineErrorf("zero data packet received")
					return
				}
				// is it 512? Are we getting more?
				if len(typedpkt.Data) == maxDataPkt {
					copy(buf[curBufInd:], typedpkt.Data)
					curBufInd += maxDataPkt
					// and back into the loop we go
				} else if len(typedpkt.Data) < maxDataPkt { // last ack
					copy(buf[curBufInd:], typedpkt.Data)
					totalWritten := curBufInd + len(typedpkt.Data)
					lapkt, err := genNextPacket(typedpkt)
					if err != nil {
						log.EngineErrorf("can't send the last ack")
					}
					r.thread.marshalWrite(lapkt)

					// it would be a good time to write it to the disk
					dwreq := r.thread.disk.NewWriteRequest(r.fname)
					hm, err := dwreq.Write(buf[:totalWritten])
					if err != nil {
						log.EngineErrorf("failed to write fname:%s to disk", r.fname)
						return
					}
					log.EngineDebugf("wrote %d bytes under fname:%s to disk", hm, r.fname)

					return
				} else { // larger than 512?
					log.EngineErrorf("datapacket larger than 512 bytes received.")
					return
				}
			default:
				log.EngineErrorf("wrong type of packet received :%+v", typedpkt)
				return
			}
			// now update our current packet so that the state machine can continue
			curpkt = rpkt

		}
	}

}
