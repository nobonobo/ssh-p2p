package main

import (
	"encoding/json"
	"log"
	"net"
	"time"

	webrtc "github.com/keroserene/go-webrtc"
	"github.com/nobonobo/rtcdc-p2p/datachan"
	"github.com/nobonobo/rtcdc-p2p/signaling"
	"github.com/nobonobo/rtcdc-p2p/signaling/client"
)

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
