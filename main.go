package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	webrtc "github.com/keroserene/go-webrtc"
)

var iceServers = []string{"stun:stun.l.google.com:19302"}

const usage = `Usage: ssh-p2p SUBCMD [options]
sub-commands:
	newkey
		new generate key of connection
	server -key="..." [-dial="127.0.0.1:22"]
		ssh server side peer mode
	client -key="..." [-listen="127.0.0.1:2222"]
		ssh client side peer mode
`

func main() {
	cmd := ""
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	var flags *flag.FlagSet
	flags = flag.NewFlagSet("", flag.ExitOnError)
	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, usage)
		flags.PrintDefaults()
		os.Exit(1)
	}
	webrtc.SetLoggingVerbosity(0)
	switch cmd {
	default:
		flags.Usage()
	case "newkey":
		key, err := UUID()
		if err != nil {
			log.Fatalln(err)
		}
		fmt.Println(key)
		os.Exit(0)
	case "server":
		var addr, key string
		flags.StringVar(&addr, "dial", "127.0.0.1:22", "dial addr = host:port")
		flags.StringVar(&key, "key", "sample", "connection key")
		if err := flags.Parse(os.Args[2:]); err != nil {
			log.Fatalln(err)
		}
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT)
		s := NewServer(addr, key, "***server***")
		go func() {
			defer s.Bye()
			for {
				if err := s.Create(); err != nil {
					log.Println(err)
				}
				time.Sleep(10 * time.Second)
			}
		}()
		s.Start()
		defer s.Stop()
		<-sig
	case "client":
		var addr, key string
		flags.StringVar(&addr, "listen", "127.0.0.1:2222", "listen addr = host:port")
		flags.StringVar(&key, "key", "sample", "connection key")
		if err := flags.Parse(os.Args[2:]); err != nil {
			log.Fatalln(err)
		}
		l, err := net.Listen("tcp", addr)
		if err != nil {
			log.Fatalln(err)
		}
		log.Println("listen:", addr)
		for {
			sock, err := l.Accept()
			if err != nil {
				log.Println(err)
				continue
			}
			id, err := UUID()
			if err != nil {
				log.Println(err)
				continue
			}
			go func() {
				defer sock.Close()
				c := NewClient(key, id)
				conn, err := c.Open()
				if err != nil {
					log.Fatalln(err)
				}
				defer c.Close()
				defer conn.Close()
				log.Println("connected:", conn)
				go io.Copy(conn, sock)
				io.Copy(sock, conn)
			}()
		}
	}
}
