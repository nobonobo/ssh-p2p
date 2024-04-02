> [!NOTE]
> This project will be archived and no longer maintained. You can use this instead.
> 
> https://github.com/rtctunnel/rtctunnel
    
# ssh-p2p
ssh p2p tunneling server and client

# connection sequence

1. ssh ---dial---> ssh-p2p client
2. ssh-p2p client <----negotiation----> ssh-p2p server
3. sshd <--dial--- ssh-p2p server

# backend protocol

- RTCDataChannel/WebRTC: https://github.com/pions/webrtc
- signaling server: https://nobo-signaling.appspot.com/

  src: [signaling/gae](https://github.com/nobonobo/ssh-p2p/signaling/gae)

thx! https://github.com/pions/webrtc

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
