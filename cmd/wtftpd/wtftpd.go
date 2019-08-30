package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"wtftpd"
	"wtftpd/log"
)

var (
	loglvl, logFile, ip string
	port                int
)

func init() {
	flag.StringVar(&loglvl, "loglvl", "error", "log levels: error/debug/trace")
	flag.StringVar(&logFile, "logfile", "wtftpd.log", "the logfile that intercepted requests are stored")
	flag.IntVar(&port, "port", 69, "daemon listening port")
	flag.StringVar(&ip, "ip", "localhost", "ip to listen on")
}

const (
	logo = `
  _      ______________          __
 | | /| / /_  __/ __/ /____  ___/ /
 | |/ |/ / / / / _// __/ _ \/ _  / 
 |__/|__/ /_/ /_/  \__/ .__/\_,_/  
                     /_/          v0.1-pre 
press ctrl-c to exit
`
)

func main() {
	flag.Parse()
	log.InitLogs(loglvl, logFile)
	ctx, cfunc := context.WithCancel(context.Background())
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	defer func() {
		signal.Stop(c)
		cfunc()
	}()
	go func() {
		select {
		case <-c:
			cfunc()
		case <-ctx.Done():
		}
	}()
	wg := &sync.WaitGroup{}
	cf := wtftpd.NewConf(ip, uint16(port))
	daemon, err := wtftpd.NewWtftpd(ctx, wg, cf)
	if err != nil {
		log.EngineFatal(err)
	}
	wg.Add(1)
	go daemon.Serve()
	fmt.Println(logo)
	wg.Wait()
}
