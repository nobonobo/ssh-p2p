# ssh-p2p
ssh p2p tunneling server and client

# connection sequence

1. ssh ---dial---> ssh-p2p client
2. ssh-p2p client <----negotiation----> ssh-p2p server
3. sshd <--dial--- ssh-p2p server

# backend protocol

- RTCDataChannel/WebRTC
- signaling server on appengine-go: https://signaling-2016.appspot.com/

  src: [signaling/server](https://github.com/nobonobo/rtcdc-p2p/tree/e50487af6b9c5aecc3f66386d4be497187f1de0e/signaling/server)

thx! https://github.com/keroserene/go-webrtc

# install

```sh
$ go get -u github.com/nobonobo/ssh-p2p
```

# usage

## server side

```sh
$ KEY = $(ssh-p2p newkey)
$ echo $KEY
xxxxxxxx-xxxx-xxxx-xxxxxxxx
$ ssh-p2p server -key=$KEY -dial=127.0.0.1:22
```

share $KEY value to client side

## client side

```sh
$ KEY=xxxxxxxx-xxxx-xxxx-xxxxxxxx
$ ssh-p2p client -key=$KEY -listen=127.0.0.1:2222
```

## client side other terminal

```sh
$ ssh -p 2222 127.0.0.1
```

**connect to server side sshd !!**
