package signaling

// URI default signaling server
const URI = "https://nobo-signaling.appspot.com"

// ConnectInfo SDP by offer or answer
type ConnectInfo struct {
	Source string `json:"source"`
	SDP    string `json:"sdp"`
}
