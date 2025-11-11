# How to run

Building docker image:
`docker build -t server-node .`

Running docker:
`docker run --rm -it \
 -v "/models/node1/:/models" \
 -p 9002:9002 \
 server-node \
 -listen "/ip4/0.0.0.0/tcp/9002" \
 -local /models \
 -remote /models/remote-models \
 -nick node2 \
 -peer /ip4/172.17.0.2/tcp/9001/p2p/12D3KooWQep4BBcvaYwzHLyVTBofUwHrSsJuQatwfx6BLiaezSqd`
