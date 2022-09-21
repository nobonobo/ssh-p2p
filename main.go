package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/nobonobo/ssh-p2p/signaling"
	"github.com/pion/ice/v2"
	"github.com/pion/webrtc/v3"
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

var (
	defaultRTCConfiguration = webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{
					"stun:stun.l.google.com:19302",
				},
			},
		},
	}
)

// Encode encodes the input in base64
// It can optionally zip the input before encoding
func Encode(obj interface{}) string {
	b, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}

	return base64.StdEncoding.EncodeToString(b)
}

// Decode decodes the input from base64
// It can optionally unzip the input after decoding
func Decode(in string, obj interface{}) {
	b, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(b, obj)
	if err != nil {
		panic(err)
	}
}

func push(dst, src, sdp string) error {
	buf := bytes.NewBuffer(nil)
	if err := json.NewEncoder(buf).Encode(signaling.ConnectInfo{
		Source: src,
		SDP:    sdp,
	}); err != nil {
		return err
	}
	resp, err := http.Post(signaling.URI+path.Join("/", "push", dst), "application/json", buf)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http failed")
	}
	return nil
}

func pull(ctx context.Context, id string) <-chan signaling.ConnectInfo {
	ch := make(chan signaling.ConnectInfo)
	var retry time.Duration
	go func() {
		faild := func() {
			if retry < 10 {
				retry++
			}
			time.Sleep(retry * time.Second)
		}
		defer close(ch)
		for {
			req, err := http.NewRequest("GET", signaling.URI+path.Join("/", "pull", id), nil)
			if err != nil {
				if ctx.Err() == context.Canceled {
					return
				}
				log.Println("get failed:", err)
				faild()
				continue
			}
			req = req.WithContext(ctx)
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				if ctx.Err() == context.Canceled {
					return
				}
				log.Println("get failed:", err)
				faild()
				continue
			}
			defer res.Body.Close()
			retry = time.Duration(0)
			var info signaling.ConnectInfo
			if err := json.NewDecoder(res.Body).Decode(&info); err != nil {
				if err == io.EOF {
					continue
				}
				if ctx.Err() == context.Canceled {
					return
				}
				log.Println("get failed:", err)
				faild()
				continue
			}
			if len(info.Source) > 0 && len(info.SDP) > 0 {
				ch <- info
			}
		}
	}()
	return ch
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
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
		key := uuid.New().String()
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
		ctx, cancel := context.WithCancel(context.Background())
		go serve(ctx, key, addr)
		<-sig
		cancel()
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
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT)
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			for {
				sock, err := l.Accept()
				if err != nil {
					log.Println(err)
					continue
				}
				go connect(ctx, key, sock)
			}
		}()
		<-sig
		cancel()
	}
}

type sendWrap struct {
	*webrtc.DataChannel
}

func (s *sendWrap) Write(b []byte) (int, error) {
	err := s.DataChannel.Send(b)
	return len(b), err
}

func serve(ctx context.Context, key, addr string) {
	log.Println("server started")
	for v := range pull(ctx, key) {
		log.Printf("info: %#v", v)
		pc, err := webrtc.NewPeerConnection(defaultRTCConfiguration)
		if err != nil {
			log.Println("rtc error:", err)
			continue
		}
		ssh, err := net.Dial("tcp", addr)
		if err != nil {
			log.Println("ssh dial filed:", err)
			pc.Close()
			continue
		}
		pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
			log.Print("pc ice state change:", state)
			if state == ice.ConnectionStateDisconnected {
				pc.Close()
				ssh.Close()
			}
		})
		pc.OnDataChannel(func(dc *webrtc.DataChannel) {
			//dc.Lock()
			dc.OnOpen(func() {
				log.Print("dial:", addr)
				io.Copy(&sendWrap{dc}, ssh)
				log.Println("disconnected")
			})
			dc.OnMessage(func(msg webrtc.DataChannelMessage) {
				_, err := ssh.Write(msg.Data)
				if err != nil {
					log.Println("ssh write failed:", err)
					pc.Close()
					return
				}
			})
			//dc.Unlock()
		})
		offer := webrtc.SessionDescription{}
		Decode(v.SDP, &offer)
		if err := pc.SetRemoteDescription(offer); err != nil {
			log.Println("rtc error:", err)
			pc.Close()
			ssh.Close()
			continue
		}
		answer, err := pc.CreateAnswer(nil)
		if err != nil {
			log.Println("rtc error:", err)
			pc.Close()
			ssh.Close()
			continue
		}
		// Create channel that is blocked until ICE Gathering is complete
		gatherComplete := webrtc.GatheringCompletePromise(pc)

		// Sets the LocalDescription, and starts our UDP listeners
		err = pc.SetLocalDescription(answer)
		if err != nil {
			panic(err)
		}
		// Block until ICE Gathering is complete, disabling trickle ICE
		// we do this because we only can exchange one signaling message
		// in a production application you should exchange ICE Candidates via OnICECandidate
		<-gatherComplete
		if err := push(v.Source, key, Encode(*pc.LocalDescription())); err != nil {
			log.Println("rtc error:", err)
			pc.Close()
			ssh.Close()
			continue
		}
	}
}

func connect(ctx context.Context, key string, sock net.Conn) {
	id := uuid.New().String()
	log.Println("client id:", id)
	pc, err := webrtc.NewPeerConnection(defaultRTCConfiguration)
	if err != nil {
		log.Println("rtc error:", err)
		return
	}
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Print("pc ice state change:", state)
	})
	dc, err := pc.CreateDataChannel("data", nil)
	if err != nil {
		log.Println("create dc failed:", err)
		pc.Close()
		return
	}
	//dc.Lock()
	dc.OnOpen(func() {
		io.Copy(&sendWrap{dc}, sock)
		pc.Close()
		log.Println("disconnected")
	})
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		_, err := sock.Write(msg.Data)
		if err != nil {
			log.Println("sock write failed:", err)
			pc.Close()
			return
		}

	})
	//dc.Unlock()
	log.Printf("DataChannel: %#v\n", dc)
	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		for v := range pull(ctx, id) {
			log.Printf("info: %#v", v)
			answer := webrtc.SessionDescription{}
			Decode(v.SDP, &answer)
			if err := pc.SetRemoteDescription(answer); err != nil {
				log.Println("rtc error:", err)
				pc.Close()
				return
			}
			return
		}
	}()
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		log.Println("create offer error:", err)
		pc.Close()
		return
	}
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	err = pc.SetLocalDescription(offer)
	if err != nil {
		panic(err)
	}

	// Block until ICE Gathering is complete, disabling trickle ICE
	// we do this because we only can exchange one signaling message
	// in a production application you should exchange ICE Candidates via OnICECandidate
	<-gatherComplete
	if err := push(key, id, Encode(*pc.LocalDescription())); err != nil {
		log.Println("push error:", err)
		pc.Close()
		return
	}
}
