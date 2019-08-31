// Package packet provides message descriptions that are exchange
// in wtftpd as well as their marshallers and unmarshallers from/to bytes.
package packet

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"wtftpd/errors"
	"wtftpd/log"
)

// TFTPType is the 2 byte opcode in the the TFTP header
type TFTPType uint16

const (
	// RRQ read request
	RRQ TFTPType = iota + 1
	// WRQ write request
	WRQ
	// DAT data
	DAT
	// ACK acknoweledgment
	ACK
	// ERR error
	ERR
)

const (
	// ModeNetASCII is ignored
	ModeNetASCII = "netascii"
	// ModeOctet is the octet encoding mode
	ModeOctet = "octet"
	// ModeMail is ignored
	ModeMail = "mail"
)

const (
	maxPktLen          = 516 // headers + a full transfer
	minPktLen          = 4   // header + data with no bytes
	minNoHdrInitPktLen = 4   // wrq/rrq with 1byte string
	maxNoHdrInitPktLen = 137 // 128 nul term fname + nul term "netascii"
)

// NetASCII is a string that has to be able to be represented
// in 8-bit ASCII
type NetASCII []byte

// NewNetASCII takes a string and returns a null terminated byte buffer of it.
func NewNetASCII(a string) NetASCII {
	retstr := fmt.Sprintf("%s\000", a)
	return []byte(retstr)
}

// returns a local variable from the mode, in order not to have to allocate a buffer
// per packet to store it.
func getMode(n []byte) (string, error) {
	const op errors.Op = "getMode"
	if len(n) == len(ModeNetASCII)+1 && bytes.Equal(n[:len(n)-1], []byte(ModeNetASCII)) {
		return ModeNetASCII, mkPacketParseError(op, "mode netascii not supported in wtftpd")
	}
	if len(n) == len(ModeOctet)+1 && bytes.Equal(n[:len(n)-1], []byte(ModeOctet)) {
		return ModeOctet, nil
	}
	if len(n) == len(ModeMail)+1 && bytes.Equal(n[:len(n)-1], []byte(ModeMail)) {
		return ModeMail, mkPacketParseError(op, "mode mail not supported in wtftpd")
	}
	return "", mkPacketParseError(op, fmt.Sprintf("unknown mode string:%v", n))
}

func (n NetASCII) String() string {
	return string(n)
}

//allocates a new buffer to hold a string.
//it makes sure the string is 7 byte ascii and that it is null
//terminated. It doesn't do any of the fancy CR/LF CR/NULL
//distinctions cause these would be silly filenames.
//this will copy the null termination too
func unmarshalNetASCII(b []byte) (NetASCII, int, error) {
	const op errors.Op = "unmarshalNetASCII"
	i := 0
	for i = range b {
		if b[i] < byte(128) && b[i] >= byte(0) {
			if b[i] == byte(0) {
				break
			}
		} else {
			return nil, i, mkPacketParseError(op, "invalid netascii character")
		}
	}
	n := make([]byte, i)
	copy(n, b)

	return NetASCII(n), i, nil
}

// Packet is satisfied by a TFTP packet. it gives the type for further unmarshals
type Packet interface {
	Type() TFTPType
	marshal() ([]byte, error)
	unmarshal([]byte) error
}

// Marshal returns the bytes of the packet or an error.
func Marshal(pkt Packet) ([]byte, error) {
	return pkt.marshal()
}

// ParsePacket determines the correct type of message from the opcode and further marshals
// that type.
func ParsePacket(a []byte) (Packet, error) {
	const op errors.Op = "ParsePacket"
	if len(a) < minPktLen {
		return nil, mkPacketParseError(op, "length of packet less than minimum length")
	}
	opcode := binary.BigEndian.Uint16(a[0:2])
	var p Packet
	switch opcode {
	case 1:
		p = new(RRQPacket)
	case 2:
		p = new(WRQPacket)
	case 3:
		p = new(DataPacket)
	case 4:
		p = new(AckPacket)
	case 5:
		p = new(ErrPacket)
	default:
		return nil, mkPacketParseError(op, "unknown opcode")
	}
	if err := p.unmarshal(a[2:]); err != nil {
		return nil, err
	}
	log.PacketTracef("Parsed packet of type:%v size:%d", p.Type(), len(a))
	return p, nil

}

// RRQPacket is the golang representation of a TFT read request
type RRQPacket struct {
	Fname NetASCII
	Mode  string
}

// Type implements the Packet interface
func (r *RRQPacket) Type() TFTPType {
	return RRQ
}

func (r *RRQPacket) marshal() ([]byte, error) {
	varlen := len(r.Fname) + len(r.Mode) + 1
	buf := make([]byte, varlen+2)
	binary.BigEndian.PutUint16(buf, uint16(r.Type()))
	copy(buf[2:], r.Fname)
	copy(buf[2+len(r.Fname):], r.Mode)
	buf[len(buf)-1] = 0 // null terminate mode.
	return buf, nil
}

func (r *RRQPacket) unmarshal(b []byte) error {
	const op errors.Op = "RRQPacket.unmarshal"
	if len(b) < minNoHdrInitPktLen {
		return mkPacketParseError(op, "Not enough bytes on read request")
	}
	if len(b) > maxNoHdrInitPktLen {
		return mkPacketParseError(op, "Too many bytes on read request")
	}
	fname, fnlen, err := unmarshalNetASCII(b)
	if err != nil {
		return err
	}
	locMode, err := getMode(b[fnlen+1:])
	if err != nil {
		return err
	}
	*r = RRQPacket{
		Fname: fname,
		Mode:  locMode,
	}

	return nil
}

// WRQPacket is the golang representation of a TFTP write request
type WRQPacket struct {
	Fname NetASCII
	Mode  string
}

// Type implements the Packet interface
func (w *WRQPacket) Type() TFTPType {
	return WRQ
}

func (w *WRQPacket) marshal() ([]byte, error) {
	varlen := len(w.Fname) + len(w.Mode) + 1
	buf := make([]byte, varlen+2)
	binary.BigEndian.PutUint16(buf, uint16(w.Type()))
	copy(buf[2:], w.Fname)
	copy(buf[2+len(w.Fname):], w.Mode)
	buf[len(buf)-1] = 0 // null terminate mode
	return buf, nil
}

func (w *WRQPacket) unmarshal(b []byte) error {
	const op errors.Op = "WRQPacket.unmarshal"
	if len(b) < minNoHdrInitPktLen {
		return mkPacketParseError(op, "Not enough bytes on write request")
	}
	if len(b) > maxNoHdrInitPktLen {
		return mkPacketParseError(op, "Too many bytes on write request")
	}
	fname, fnlen, err := unmarshalNetASCII(b)
	if err != nil {
		return err
	}
	locMode, err := getMode(b[fnlen+1:])
	if err != nil {
		return err
	}
	*w = WRQPacket{
		Mode:  locMode,
		Fname: fname,
	}

	return nil
}

// DataPacket is the golang representation of a TFTP data packet
type DataPacket struct {
	BlockNumber uint16
	Data        []byte
}

// Type implements the Packet interface
func (d *DataPacket) Type() TFTPType {
	return DAT
}

func (d *DataPacket) marshal() ([]byte, error) {
	buf := make([]byte, len(d.Data)+4)
	binary.BigEndian.PutUint16(buf, uint16(d.Type()))
	binary.BigEndian.PutUint16(buf[2:], d.BlockNumber)
	copy(buf[4:], d.Data)
	return buf, nil
}

func (d *DataPacket) unmarshal(a []byte) error {
	bnum := binary.BigEndian.Uint16(a[0:2])
	locData := make([]byte, len(a[2:]))
	copy(locData, a[2:])
	*d = DataPacket{
		Data:        locData,
		BlockNumber: bnum,
	}
	return nil
}

// AckPacket is the golang representation of a TFTP Acknowledge packet
type AckPacket struct {
	BlockNumber uint16
}

// Type implements the Packet interface
func (a *AckPacket) Type() TFTPType {
	return ACK
}

func (a *AckPacket) marshal() ([]byte, error) {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint16(buf, uint16(a.Type()))
	binary.BigEndian.PutUint16(buf[2:], a.BlockNumber)
	return buf, nil
}

func (a *AckPacket) unmarshal(b []byte) error {
	const op errors.Op = "AckPacket.unmarshal"
	if len(b) != 2 {
		return mkPacketParseError(op, "ack packet with more bytes")
	}
	*a = AckPacket{
		BlockNumber: binary.BigEndian.Uint16(b),
	}
	return nil
}

// ErrPacket is the golang representation of a TFTP error packet
type ErrPacket struct {
	ErrCode uint16
	Msg     NetASCII
}

// Type implements the Packet interface
func (e *ErrPacket) Type() TFTPType {
	return ERR
}

func (e *ErrPacket) marshal() ([]byte, error) {
	varlen := len(e.Msg) + 1
	buf := make([]byte, varlen+4)
	binary.BigEndian.PutUint16(buf, uint16(e.Type()))
	binary.BigEndian.PutUint16(buf[2:], e.ErrCode)
	copy(buf[4:], e.Msg)
	buf[len(buf)-1] = 0
	return buf, nil
}

func (e *ErrPacket) unmarshal(b []byte) error {
	errmsg, _, err := unmarshalNetASCII(b[2:])
	if err != nil {
		return err
	}
	*e = ErrPacket{
		Msg:     errmsg,
		ErrCode: binary.BigEndian.Uint16(b[:2]),
	}
	return nil
}

// Type implements the Packet interface
func mkPacketParseError(op errors.Op, a string) error {
	return errors.E(op, fmt.Sprintf("can't parse packet:%s", a))
}
