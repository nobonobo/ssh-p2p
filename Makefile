DST := localhost:22

.PHONY: deploy test

deploy:
	gcloud app deploy signaling/gae

build:
	go build .

server:
	./ssh-p2p server -key=6ee87ebb-2938-47f9-8577-e8fd4aa3988c -dial=$(DST)

client:
	./ssh-p2p client -key=6ee87ebb-2938-47f9-8577-e8fd4aa3988c -listen=localhost:2222
