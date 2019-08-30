package disk

import (
	"bytes"
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"
)

func getRandFname16() string {
	b := [16]byte{}
	rand.Read(b[:])
	return string(b[:])
}
func getRandBytes512() []byte {
	b := [512]byte{}
	rand.Read(b[:])
	return b[:]
}
func getCtxWg() (*sync.WaitGroup, context.Context, context.CancelFunc) {
	ctx, cf := context.WithCancel(context.Background())
	return &sync.WaitGroup{}, ctx, cf
}

func writeReadOne(md *MapDisk, fname string, in []byte, t testing.TB) {
	req := md.NewWriteRequest(fname)
	hmi, err := req.Write(in)
	if err != nil {
		t.Errorf("writefile error:%s", err)
	}
	//make receipent buffer that we now know is of the same size
	dat := make([]byte, len(in))
	req = md.NewReadRequest(fname)
	hmo, err := req.Read(dat)
	if err != nil {
		t.Errorf("readfile error:%s", err)
	}
	if hmi != hmo && !bytes.Equal(dat, in) {
		t.Errorf("readfile byte mismatch. sent:%v got:%v", in, dat)
	}
}

func TestMapDisk(t *testing.T) {
	t.Run("explicit cancellation", func(t *testing.T) {
		wg, ctx, cf := getCtxWg()
		NewMapDisk(ctx, wg)
		cf()
		deadchan := make(chan bool)
		go func() {
			wg.Wait()
			deadchan <- true
		}()
		go func() {
			<-time.After(1 * time.Second)
			deadchan <- false
		}()
		if !<-deadchan {
			t.Error("context cancel on disk failed")
		}
		<-deadchan //read the one second timer too
	})
	t.Run("send/receive one", func(t *testing.T) {
		wg, ctx, _ := getCtxWg()
		md := NewMapDisk(ctx, wg)
		writeReadOne(md, getRandFname16(), getRandBytes512(), t)
	})
}

func BenchmarkMapDisk(b *testing.B) {
	b.Run("multiple one send/receives", func(b *testing.B) {
		wg, ctx, _ := getCtxWg()
		md := NewMapDisk(ctx, wg)
		for n := 0; n < b.N; n++ {
			writeReadOne(md, getRandFname16(), getRandBytes512(), b)
		}
	})
}
