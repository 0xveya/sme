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
		log.Fatal("Usage: sudo ./snb <pid_a> <pid_b>")
	}

	pidA, err := strconv.Atoi(os.Args[1])
	if err != nil {
		log.Fatalf("invalid pid_a: %v", err)
	}

	pidB, err := strconv.Atoi(os.Args[2])
	if err != nil {
		log.Fatalf("invalid pid_b: %v", err)
	}

	hostA := "vethA-host"
	hostB := "vethB-host"

	containerIfA := "eth0"
	containerIfB := "eth0"

	mtu := 1500

	usePcap := true
	immediateMode := true

	bridge := libsnb.NewBridge()

	libsnb.CleanupLink(hostA)
	libsnb.CleanupLink(hostB)

	defer func() {
		fmt.Println("\nCleaning up...")

		if err := bridge.Close(); err != nil {
			log.Printf("bridge shutdown error: %v", err)
		}

		libsnb.CleanupLink(hostA)
		libsnb.CleanupLink(hostB)
	}()

	sigChan := make(chan os.Signal, 1)

	signal.Notify(
		sigChan,
		os.Interrupt,
		syscall.SIGTERM,
		syscall.SIGINT,
	)

	fmt.Printf(
		"Starting bridge between PID %d and PID %d...\n",
		pidA,
		pidB,
	)

	if err := bridge.Connect(
		pidA,
		hostA,
		containerIfA,
		mtu,
	); err != nil {
		fmt.Printf("failed to connect A: %v", err)
		return
	}

	if err := bridge.Connect(
		pidB,
		hostB,
		containerIfB,
		mtu,
	); err != nil {
		fmt.Printf("failed to connect B: %v", err)
		return
	}

	fmt.Println("Binding packet transports...")

	if err := bridge.Bind(
		hostA,
		mtu,
		usePcap,
		immediateMode,
	); err != nil {
		fmt.Printf("failed to bind %s: %v", hostA, err)
		return
	}

	if err := bridge.Bind(
		hostB,
		mtu,
		usePcap,
		immediateMode,
	); err != nil {
		fmt.Printf("failed to bind %s: %v", hostB, err)
		return
	}

	fmt.Println("Starting forwarding engine...")

	bridge.Start(
		hostA,
		hostB,
		mtu,
	)

	fmt.Println("Bridge running.")
	fmt.Println("Press Ctrl+C to stop.")

	sig := <-sigChan

	fmt.Printf("\nReceived signal: %s\n", sig.String())
	fmt.Println("Stopping bridge...")
}
