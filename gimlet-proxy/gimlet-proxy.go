package main

import (
	"context"
	"flag"
	"github.com/Cyberax/gimlet"
	"github.com/Cyberax/gimlet/pierce"
	"github.com/aws/aws-sdk-go-v2/config"
	"io"
	"log"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"time"
)

func runProxy(listener *net.TCPListener, channel *gimlet.Channel, doneChan chan bool) {
	defer close(doneChan)

	for {
		tcp, err := listener.AcceptTCP()
		if err != nil {
			log.Printf("Failed to accept TCP connection, err=%v", err)
			break
		}
		stream, err := channel.OpenStream()
		if err != nil {
			log.Printf("Failed to connect to the target host, err=%v", err)
			break
		}
		go transmit(stream, tcp)
		go transmit(tcp, stream)
	}
}

func transmit(in io.ReadCloser, out io.WriteCloser) {
	defer func() {
		_ = in.Close()
		_ = out.Close()
	}()

	buf := make([]byte, 65536)
	for {
		n, err := in.Read(buf)
		if err != nil || n <= 0 {
			break
		}
		_, err = out.Write(buf[:n])
		if err != nil {
			break
		}
	}
}

func main() {
	var debug bool
	var profile, instanceId, targetHost string
	var targetPort uint
	var localPort uint

	flag.BoolVar(&debug, "debug", false, "Debug mode")
	flag.StringVar(&profile, "profile", "", "AWS profile")
	flag.StringVar(&instanceId, "instance-id", "", "EC2 Instance ID")

	flag.StringVar(&targetHost, "target-host", "", "Host to connect (empty for localhost)")
	flag.UintVar(&targetPort, "target-port", 22, "The port to connect to")

	flag.UintVar(&localPort, "listen-port", 2222, "The listen port")

	flag.Parse()

	if instanceId == "" {
		_, _ = os.Stderr.WriteString("Error: no EC2 Instance ID specified\n")
		os.Exit(1)
	}

	// Create the listener
	listener, err := net.ListenTCP("tcp", net.TCPAddrFromAddrPort(
		netip.AddrPortFrom(netip.MustParseAddr("::"), uint16(localPort))))
	if err != nil {
		_, _ = os.Stderr.WriteString("Error: failed to open listen port, err=" + err.Error() + "\n")
		os.Exit(2)
	}
	defer func() { _ = listener.Close() }()

	ctx := context.Background()

	cfg, err := config.LoadDefaultConfig(ctx, config.WithSharedConfigProfile(profile))
	if err != nil {
		_, _ = os.Stderr.WriteString("Error: failed to load the AWS profile, err=" + err.Error() + "\n")
		os.Exit(1)
	}

	connInfo, err := pierce.PierceVpcVeil(ctx, cfg, instanceId, targetHost, uint16(targetPort))
	if err != nil {
		_, _ = os.Stderr.WriteString("Error: failed to create SSM connection, err=" + err.Error() + "\n")
		os.Exit(2)
	}

	opts := gimlet.DefaultChannelOptions()
	if debug {
		opts = gimlet.DebugChannelOptions(false)
	}

	channel, err := gimlet.NewChannel(ctx, connInfo.Endpoint, connInfo.Token, opts)
	if err != nil {
		_, _ = os.Stderr.WriteString("Error: failed establish the Gimlet connection, err=" + err.Error() + "\n")
		os.Exit(2)
	}
	defer channel.Shutdown(ctx)

	doneChan := make(chan bool)
	go runProxy(listener, channel, doneChan)

	go printStats(channel)

	// Listen for SIGINT
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt)
	select {
	case <-c:
	case <-doneChan:
	}

	_ = listener.Close()
}

func printStats(channel *gimlet.Channel) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-t.C:
		}
		st := channel.GetStats().String()
		log.Printf("Stats: %s\n", st)
	}
}
