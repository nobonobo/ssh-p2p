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

	"github.com/nobonobo/p2pfw/peerconn"
	"github.com/nobonobo/p2pfw/signaling/client"
	"github.com/nobonobo/webrtc"
)

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

	switch cmd {
	default:
		flags.Usage()
	case "newkey":
		key, err := client.UUID()
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
		defer serve(key, addr)()
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
			go connect(key, sock)
		}
	}
}

func serve(key, addr string) func() error {
	dial := new(client.Config)
	dial.RoomID = key
	dial.UserID = "***server***"
	dial.URL = "wss://signaling.arukascloud.io/ws"

	stun, err := peerconn.GetDefaultStunHosts()
	if err != nil {
		log.Fatalln(err)
	}
	config := webrtc.NewConfiguration()
	config.AddIceServer(stun)
	node, err := peerconn.NewNode(dial, config)
	if err != nil {
		log.Fatalln(err)
	}
	node.OnLeave = func(member string) {
		log.Println("leave:", member)
		node.Clients.Del(member)
	}
	node.OnPeerConnection = func(dest string, conn *peerconn.Conn) error {
		dc, err := conn.CreateDataChannel("default")
		if err != nil {
			return err
		}
		dc.OnOpen(func() {
			go func() {
				c := peerconn.NewDCConn(dc)
				defer c.Close()
				ssh, err := net.Dial("tcp", addr)
				if err != nil {
					log.Println("dial failed:", err)
					return
				}
				defer ssh.Close()
				log.Println("connected:", dest)
				go io.Copy(ssh, c)
				io.Copy(c, ssh)
			}()
		})
		dc.OnClose(func() {
			log.Println("disconnected:", dest)
			dc.Close()
		})
		return nil
	}
	if err := node.Start(true); err != nil {
		log.Fatalln(err)
	}
	return node.Close
}

func connect(key string, sock net.Conn) {
	id, err := client.UUID()
	if err != nil {
		log.Fatalln(err)
	}

	dial := new(client.Config)
	dial.RoomID = key
	dial.UserID = id
	dial.URL = "wss://signaling.arukascloud.io/ws"

	stun, err := peerconn.GetDefaultStunHosts()
	if err != nil {
		log.Fatalln(err)
	}
	config := webrtc.NewConfiguration()
	config.AddIceServer(stun)
	node, err := peerconn.NewNode(dial, config)
	if err != nil {
		log.Fatalln(err)
	}
	if err := node.Start(false); err != nil {
		log.Fatalln(err)
	}
	members, err := node.Members()
	if err != nil {
		log.Fatalln(err)
	}
	conn, err := node.Connect(members.Owner)
	if err != nil {
		log.Fatalln(err)
	}
	conn.OnDataChannel(func(dc *webrtc.DataChannel) {
		go func() {
			defer func() {
				if err := node.Close(); err != nil {
					log.Println(err)
				}
			}()
			log.Println("data channel open:", dc)
			defer log.Println("data channel close:", dc)
			c := peerconn.NewDCConn(dc)
			go func() {
				defer func() {
					if err := c.Close(); err != nil {
						log.Println(err)
					}
				}()
				if _, err := io.Copy(c, sock); err != nil {
					log.Println(err)
				}
			}()
			defer func() {
				if err := sock.Close(); err != nil {
					log.Println(err)
				}
			}()
			if _, err := io.Copy(sock, c); err != nil {
				log.Println(err)
			}
		}()
	})
}
