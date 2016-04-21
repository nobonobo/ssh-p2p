package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	webrtc "github.com/keroserene/go-webrtc"
	"github.com/nobonobo/rtcdc-p2p/datachan"
	"github.com/nobonobo/rtcdc-p2p/signaling"
	"github.com/nobonobo/rtcdc-p2p/signaling/client"
)

var iceServers = []string{"stun:stun.l.google.com:19302"}

// Server ...
type Server struct {
	addr string
	*client.Client
	members map[string]*datachan.Connection
}

// NewServer ...
func NewServer(addr, room, id string) *Server {
	s := new(Server)
	s.addr = addr
	s.Client = client.New(room, id, s.dispatch)
	s.members = map[string]*datachan.Connection{}
	return s
}

// Send ...
func (s *Server) Send(to string, v interface{}) error {
	log.Printf("send: %T to %s\n", v, to)
	m := signaling.New(s.ID(), to, v)
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	s.Client.Send(b)
	return nil
}

func (s *Server) dispatch(b []byte) {
	var m *signaling.Message
	if err := json.Unmarshal(b, &m); err != nil {
		log.Println(err)
		return
	}
	if m.Sender == s.ID() {
		return
	}
	if m.To != "" && m.To != s.ID() {
		return
	}
	value, err := m.Get()
	if err != nil {
		log.Println(err)
		return
	}
	log.Printf("recv: %T from %s\n", value, m.Sender)
	switch v := value.(type) {
	case *signaling.Request:
		if conn := s.members[m.Sender]; conn != nil {
			conn.Close()
		}
		conn, err := datachan.New(iceServers)
		if err != nil {
			log.Println("datachan new failed:", err)
			delete(s.members, m.Sender)
			return
		}
		conn.OnDataChannel = func(channel *webrtc.DataChannel) {
			conn, err := net.Dial("tcp", s.addr)
			if err != nil {
				log.Println("dial failed:", err)
				return
			}
			defer conn.Close()
			c := datachan.NewConn(channel)
			defer c.Close()
			go io.Copy(conn, c)
			io.Copy(c, conn)
		}
		offer, err := conn.Offer()
		if err != nil {
			log.Println("offer failed:", err)
			delete(s.members, m.Sender)
			return
		}
		if err := s.Send(m.Sender, &signaling.Offer{Description: offer.Serialize()}); err != nil {
			log.Println("send failed:", err)
			delete(s.members, m.Sender)
			return
		}
		log.Println("offer completed:", m.Sender)
		s.members[m.Sender] = conn

	case *signaling.Offer:
	case *signaling.Answer:
		conn := s.members[m.Sender]
		if conn == nil {
			log.Println("connection failed:", m.Sender)
			return
		}
		sdp := webrtc.DeserializeSessionDescription(v.Description)
		if sdp == nil {
			log.Println("desirialize sdp failed", v.Description)
			return
		}
		if err := conn.SetRemoteDescription(sdp); err != nil {
			log.Println("answer set failed:", err)
			delete(s.members, m.Sender)
			conn.Close()
			return
		}
		ices := conn.IceCandidates()
		log.Println("ices:", len(ices))
		for _, ice := range ices {
			msg := &signaling.Candidate{
				Candidate:     ice.Candidate,
				SdpMid:        ice.SdpMid,
				SdpMLineIndex: ice.SdpMLineIndex,
			}
			log.Printf("candidate: %q\n", ice.Candidate)
			if err := s.Send(m.Sender, msg); err != nil {
				log.Println(err)
				return
			}
			time.Sleep(100 * time.Microsecond)
		}
	case *signaling.Candidate:
		conn := s.members[m.Sender]
		if conn == nil {
			log.Println("connection failed:", m.Sender)
			return
		}
		ice := webrtc.DeserializeIceCandidate(string(m.Value))
		if err := conn.AddIceCandidate(*ice); err != nil {
			log.Println("add ice failed:", err)
		}
	}
}

// Client ...
type Client struct {
	*client.Client
	conn *datachan.Connection
}

// NewClient ...
func NewClient(room, id string) *Client {
	c := new(Client)
	c.Client = client.New(room, id, c.dispatch)
	return c
}

// Open ...
func (c *Client) Open() (net.Conn, error) {
	con, err := datachan.New(iceServers)
	if err != nil {
		return nil, err
	}
	c.conn = con
	if err := c.Join(); err != nil {
		return nil, err
	}
	defer c.Bye()
	complete := make(chan net.Conn, 1)
	c.conn.OnDataChannel = func(channel *webrtc.DataChannel) {
		complete <- datachan.NewConn(channel)
	}
	c.Start()
	defer c.Stop()
	time.Sleep(time.Second)
	if err := c.Send("", &signaling.Request{}); err != nil {
		return nil, err
	}
	channel := <-complete
	return channel, nil
}

// Close ...
func (c *Client) Close() {

}

// Send ...
func (c *Client) Send(to string, v interface{}) error {
	log.Println("send to:", to, v)
	m := signaling.New(c.ID(), to, v)
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	c.Client.Send(b)
	return nil
}

func (c *Client) dispatch(b []byte) {
	var m *signaling.Message
	if err := json.Unmarshal(b, &m); err != nil {
		log.Println(err)
		return
	}
	if m.Sender == c.ID() {
		return
	}
	if m.To != c.ID() {
		return
	}
	value, err := m.Get()
	if err != nil {
		log.Println(err)
		return
	}
	log.Printf("recv: %T from %s\n", value, m.Sender)
	switch v := value.(type) {
	case *signaling.Request:
	case *signaling.Offer:
		sdp := webrtc.DeserializeSessionDescription(v.Description)
		if sdp == nil {
			log.Println("desirialize sdp failed", v.Description)
			return
		}
		answer, err := c.conn.Answer(sdp)
		if err != nil {
			log.Println(err)
			return
		}
		if err := c.Send(m.Sender, &signaling.Answer{Description: answer.Serialize()}); err != nil {
			log.Println(err)
			return
		}
		ices := c.conn.IceCandidates()
		log.Println("ices:", len(ices))
		for _, ice := range ices[2:] {
			msg := &signaling.Candidate{
				Candidate:     ice.Candidate,
				SdpMid:        ice.SdpMid,
				SdpMLineIndex: ice.SdpMLineIndex,
			}
			log.Printf("candidate: %q\n", ice.Candidate)
			if err := c.Send(m.Sender, msg); err != nil {
				log.Println(err)
				return
			}
			time.Sleep(100 * time.Microsecond)
		}
	case *signaling.Answer:
	case *signaling.Candidate:
		ice := webrtc.DeserializeIceCandidate(string(m.Value))
		if err := c.conn.AddIceCandidate(*ice); err != nil {
			log.Println(err)
		}
	}
}

func main() {
	var client bool
	var addr, room, id string
	flag.BoolVar(&client, "client", false, "client mode")
	flag.StringVar(&addr, "addr", "127.0.0.1:22", "host:port")
	flag.StringVar(&room, "room", "sample", "name of room")
	flag.StringVar(&id, "id", "**master**", "name of id")
	flag.Parse()
	if id == "" {
		log.Fatalln("id must set unique")
	}
	webrtc.SetLoggingVerbosity(0)
	if client {
		// client mode
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
			go func() {
				c := NewClient(room, id)
				conn, err := c.Open()
				if err != nil {
					log.Fatalln(err)
				}
				log.Println("connected:", conn)
				defer c.Close()
				defer conn.Close()
				go io.Copy(conn, sock)
				io.Copy(sock, conn)
			}()
		}
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT)
	s := NewServer(addr, room, id)
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
}
