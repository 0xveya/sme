package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/0xveya/sme/internal/libsnb"
)

func main() {
	if len(os.Args) < 3 {
		log.Fatal("Usage: sudo ./bridge <pid_a> <pid_b>")
	}

	pidA, _ := strconv.Atoi(os.Args[1])
	pidB, _ := strconv.Atoi(os.Args[2])

	hostA := "vethA-host"
	hostB := "vethB-host"
	mtu := 1500

	bridge := &libsnb.Bridge{
		Connections: make(map[string]*libsnb.Endpoint),
	}

	libsnb.CleanupLink(hostA)
	libsnb.CleanupLink(hostB)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	defer func() {
		fmt.Println("\nCleaning up interfaces...")
		libsnb.CleanupLink(hostA)
		libsnb.CleanupLink(hostB)
	}()

	fmt.Printf("Starting bridge between PID %d and PID %d...\n", pidA, pidB)

	err := bridge.Connect(pidA, hostA, "eth0", mtu)
	if err != nil {
		log.Fatalf("Failed to connect A: %v", err)
	}

	err = bridge.Connect(pidB, hostB, "eth0", mtu)
	if err != nil {
		log.Fatalf("Failed to connect B: %v", err)
	}

	fmt.Println("Binding raw sockets...")
	if err := bridge.Bind(hostA, mtu); err != nil {
		log.Fatalf("Failed to bind %s: %v", hostA, err)
	}
	if err := bridge.Bind(hostB, mtu); err != nil {
		log.Fatalf("Failed to bind %s: %v", hostB, err)
	}

	fmt.Println("Links established. Starting data plane...")
	bridge.Start(hostA, hostB, mtu)

	fmt.Println("Bridge is running. Press Ctrl+C to stop.")

	<-sigChan
	fmt.Println("Shutting down...")
}
