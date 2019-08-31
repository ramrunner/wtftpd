package wtftpd

import (
	"bytes"
	"context"
	"crypto/rand"
	"io/ioutil"
	"net"
	"sync"
	"testing"
	"wtftpd/log"
)

var (
	highSrvConf = &Conf{
		ipstr: "localhost",
		port:  6969,
	}
)

func getBigData() string {
	b := [1 << 26]byte{}
	rand.Read(b[:])
	return string(b[:]) // terrible terrible just terrible :)
}

func TestEngine(t *testing.T) {
	inputs := []struct {
		fname     string
		indat     string
		shoulderr bool
	}{{"test1", "testtesttest", false},
		{"test2", "TestTestTest", false},
		{"Κύϰλωψ, τῆ, πίε οἶνον, ἐπεὶ ϕάγες ἀνδρόμεα ϰρέα", "wut", true},
		{"longcat", getBigData(), true},
	}
	t.Run("simple cli and srv write", func(t *testing.T) {
		ctx, cf := context.WithCancel(context.Background())
		wg := &sync.WaitGroup{}
		log.InitLogs("debug", ioutil.Discard)
		srv, err := NewWtftpd(ctx, wg, highSrvConf)
		if err != nil {
			t.Error(err)
		}
		wg.Add(1)
		go srv.Serve()
		for i := range inputs {
			clireq := clirequest{
				request: request{
					rtype: reqWrite,
					fname: inputs[i].fname,
				},
				in: bytes.NewReader([]byte(inputs[i].indat)),
			}
			errc := make(chan error)
			wg.Add(1)
			go newWtftpdCli(ctx, wg, srv.mainThread.addr, errc, clireq)
			for e := range errc {
				if ne, ok := e.(net.Error); ok && ne.Timeout() {
					break
				}
				if !inputs[i].shoulderr {
					t.Errorf("fail because got error from client:%s", e)
				} else {
					t.Logf("correctly caught err:%s", e)
				}
			}
			var resbuf bytes.Buffer
			rclireq := clirequest{
				request: request{
					rtype: reqRead,
					fname: inputs[i].fname,
				},
				out: &resbuf,
			}
			errc = make(chan error)
			wg.Add(1)
			go newWtftpdCli(ctx, wg, srv.mainThread.addr, errc, rclireq)
			for e := range errc {

				if ne, ok := e.(net.Error); ok && ne.Timeout() {
					break
				}
				if !inputs[i].shoulderr {
					t.Errorf("fail because got error from client:%s", e)
				} else {
					t.Logf("correctly caught err:%s", e)
				}
			}
			t.Logf("resbuf has:%v", resbuf.Bytes())
			if bytes.Equal(resbuf.Bytes(), []byte(inputs[i].indat)) {
				t.Log("succesfully wrote and retrieved a file")
			} else if !bytes.Equal(resbuf.Bytes(), []byte(inputs[i].indat)) && inputs[i].shoulderr {
				t.Log("succesfully the malformed input was not written")
			} else if inputs[i].shoulderr && bytes.Equal(resbuf.Bytes(), []byte(inputs[i].indat)) {
				t.Errorf("input %d should fail", i)
			}
		}
		cf()
		wg.Wait()

	})
}
