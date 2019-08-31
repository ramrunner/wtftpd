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
	loglvl, logFile, ip, dir string
	port                     int
)

func init() {
	flag.StringVar(&loglvl, "loglvl", "error", "log levels: error/debug/trace")
	flag.StringVar(&logFile, "logfile", "wtftpd.log", "the logfile that intercepted requests are stored")
	flag.IntVar(&port, "port", 69, "daemon listening port")
	flag.StringVar(&ip, "ip", "localhost", "ip to listen on")
	flag.StringVar(&dir, "dir", ".", "directory to scan for files to load")
}

const (
	logo = `
  _      ______________          __
 | | /| / /_  __/ __/ /____  ___/ /
 | |/ |/ / / / / _// __/ _ \/ _  / 
 |__/|__/ /_/ /_/  \__/ .__/\_,_/  
                     /_/          v0.1
press ctrl-c to exit
`
)

func main() {
	flag.Parse()
	f, err := os.OpenFile(logFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		fmt.Printf("can't open log file:%s %s", logFile, err)
		os.Exit(1)
	}
	defer f.Close()
	log.InitLogs(loglvl, f)
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
	cf := wtftpd.NewConf(ip, uint16(port), dir)
	daemon, err := wtftpd.NewWtftpd(ctx, wg, cf)
	if err != nil {
		log.EngineFatal(err)
	}
	wg.Add(1)
	go daemon.Serve()
	fmt.Println(logo)
	wg.Wait()
}
