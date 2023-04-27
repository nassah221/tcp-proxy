package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Server struct {
	roundRobinMutex sync.Mutex
	wg              sync.WaitGroup
	dialer          net.Dialer
	targetLookup    map[int]*App
	apps            Apps
}

func NewServer(apps Apps) *Server {
	// init lookup map for app targets so we don't have to loop over
	// all of the apps to get a matching port to return a target URL
	targetLookup := make(map[int]*App)
	for i := range apps {
		for port := range apps[i].Ports {
			targetLookup[port] = &apps[i]
		}
	}

	return &Server{
		apps: apps,
		dialer: net.Dialer{
			Timeout: 3 * time.Second,
		},
		targetLookup: targetLookup,
	}
}

func (s *Server) Start(ctx context.Context) error {
	log.Println("starting server")
	for _, app := range s.apps {
		app := app
		name := app.Name
		for port := range app.Ports {
			port := port
			s.wg.Add(1)
			go func() {
				if err := s.listen(ctx, name, fmt.Sprintf("localhost:%d", port)); err != nil {
					log.Println(err)
				}
			}()
		}
	}

	s.wg.Wait()

	log.Println("all listeners returned")
	return nil
}

func (s *Server) listen(ctx context.Context, name, addr string) error {
	defer s.wg.Done()

	log.Printf("starting listener on %s", addr)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	newConnChan := make(chan net.Conn)

	for {
		go func(l net.Listener) {
			defer func() {
				if closeErr := listener.Close(); closeErr != nil {
					log.Println(err)
				}
			}()
			// we don't have to close(newConnChan) in a defer statement
			// since the listener in not looking for a channel close in any case
			// the open channel will be garbage collection when no longer needed
			// https://stackoverflow.com/a/8593986
			for {
				conn, err := l.Accept()
				if err != nil {
					log.Printf("listener: %v", err)
					break
				}

				newConnChan <- conn
			}
		}(listener)
		select {
		case <-ctx.Done():
			log.Println("received done signal on listener")
			// send the error for logging
			return fmt.Errorf("closing listener %s on %s", name, addr)
		case c := <-newConnChan:
			go func() {
				if err := s.serve(ctx, c); err != nil {
					log.Println(err)
				}
			}()
		}
	}
}

func (s *Server) serve(ctx context.Context, conn net.Conn) error {
	defer conn.Close()

	addr := conn.LocalAddr().String()
	log.Printf("new connection %s -> %s", addr, conn.RemoteAddr())
	split := strings.Split(addr, ":")
	port, _ := strconv.Atoi(split[1])

	targetConn, targetAddr, err := s.connect(ctx, port, 5)
	if err != nil {
		log.Printf("cannot connect to target: %v", err)
		return err
	}
	defer func() {
		// this is for the weird behavior with net.Conn being nil if target host is unreachable
		if targetConn != nil {
			targetConn.Close()
		}
	}()

	log.Printf("connected to target %s", targetAddr)

	errClientWrite := make(chan error, 1)
	errTargetWrite := make(chan error, 1)

	// we might want to read the client connection into a buffer on successful connection
	// since the following goroutines are not guaranteed to start immediately. If the client
	// starts sending data immediately it may be dropped until the tunnel is open
	go tunnel(conn, targetConn, errClientWrite)
	go tunnel(targetConn, conn, errTargetWrite)

	select {
	case <-ctx.Done():
		log.Println("received done signal on serve")
	case err := <-errClientWrite:
		if err != nil {
			log.Printf("error writing to client: %v", err)
		}
	case err := <-errTargetWrite:
		if err != nil {
			log.Printf("error writing to target: %v", err)
		}
	}
	log.Printf("closing tunnel: %s <-> %s", conn.RemoteAddr().String(), targetAddr)

	return nil
}

var ErrTargetUnavailable = errors.New("target is unavailable")

func (s *Server) connect(ctx context.Context, port, retries int) (net.Conn, string, error) {
	ticker := time.NewTicker(time.Second * 1)
	defer ticker.Stop()

	var (
		try        int
		targetAddr string
	)

	targetAddr = s.TargetURL(port)
	if targetAddr == "" {
		return nil, "", ErrTargetUnavailable
	}

	// this select statement is to avoid waiting on the ticker for the first tick
	// we want to try to dial the target address immediately and in case of failure
	// attempt retry on each tick
	select {
	case <-ctx.Done():
		return nil, "", ctx.Err()
	default:
		conn, err := net.Dial("tcp", targetAddr)
		if err != nil {
			try++
			break // select
		}
		return conn, targetAddr, nil
	}

outer:
	for {
		select {
		case <-ctx.Done():
			return nil, "", ctx.Err()
		case <-ticker.C:
			if try == retries {
				break outer
			}

			targetAddr = s.TargetURL(port)
			if targetAddr == "" {
				return nil, "", ErrTargetUnavailable
			}

			conn, err := s.dial(targetAddr)
			if err != nil {
				try++
				continue
			}
			return conn, targetAddr, nil
		}
	}

	return nil, "", ErrTargetUnavailable
}

func (s *Server) dial(targetAddr string) (net.Conn, error) {
	conn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		log.Printf("target %s dial: %v", targetAddr, err)
		return nil, err
	}

	return conn, nil
}

func tunnel(dst, src net.Conn, errChan chan error) {
	defer func() {
		dst.Close()
		src.Close()
		close(errChan)
	}()

	for {
		// blocking call - wait until it returns
		// we could handle io.EOF or net.ErrorClosed errors in a special way
		// but to keep it simple just terminate the loop
		//
		// in a production system, this probably wouldn't return outright in case of an error
		// may be we could implement retries for resilience
		// io.Copy by default uses a 32KB buffer
		// we may as well not use io.Copy here and allocate a buffer to implement rate-limiting
		// by using tcp tuning - throughput = bps/rtt
		_, err := io.Copy(dst, src)

		// send error for logging in parent goroutine
		// and exit
		errChan <- err
		break
	}
}

// TargetURL returns one of the target URLs for the specified port
// in a round-robin fashion
func (s *Server) TargetURL(port int) string {
	s.roundRobinMutex.Lock()
	defer s.roundRobinMutex.Unlock()

	if app, ok := s.targetLookup[port]; ok {
		if app.Current == len(app.Targets) {
			app.Current %= len(app.Targets)
		}

		// no more targets to dial
		if app.Current == 1 && len(app.Targets) == 1 {
			log.Println("app is configured with only one target addr")
			return ""
		}

		target := app.Targets[app.Current]
		app.Current++
		return target
	}

	return ""
}
