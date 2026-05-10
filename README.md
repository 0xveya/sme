`sudo podman run --name a --rm -it --cap-add=NET_ADMIN --net=none alpine`
`sudo podman run --name b --rm -it --cap-add=NET_ADMIN --net=none alpine`

`sudo go run ./cmd/snb/ (sudo podman inspect -f '{{.State.Pid}}' a) (sudo podman inspect -f '{{.State.Pid}}' b)`

`ip a add dev eth0 10.0.0.1/24`
`ip a add dev eth0 10.0.0.2/24`

ping and profit
