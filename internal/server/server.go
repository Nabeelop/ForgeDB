package server

import (
	"bufio"
	"fmt"
	"net"
	"strings"

	"forgedb/internal/storage"
)

// Server coordinates the TCP server interface.
type Server struct {
	db       *storage.DB
	listener net.Listener
}

// NewServer configures a new Server instance listening on a specified address.
func NewServer(addr string, db *storage.DB) (*Server, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to bind to address %s: %w", addr, err)
	}
	return &Server{
		db:       db,
		listener: listener,
	}, nil
}

// Start starts listening and block-accepting network client connections.
func (s *Server) Start() error {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return err
		}
		go s.handleConnection(conn)
	}
}

// Close shuts down the network listener.
func (s *Server) Close() error {
	return s.listener.Close()
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	// Send initial greeting on connection
	_, _ = writer.WriteString("Welcome to ForgeDB Server!\n")
	_ = writer.Flush()

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		line = strings.TrimSpace(line)
		parts := strings.SplitN(line, " ", 3)
		if len(parts) == 0 || parts[0] == "" {
			continue
		}

		cmd := strings.ToUpper(parts[0])
		switch cmd {
		case "GET":
			if len(parts) < 2 {
				_, _ = writer.WriteString("ERR: GET requires a key\n")
				_ = writer.Flush()
				continue
			}
			key := []byte(parts[1])
			val, err := s.db.Get(key)
			if err != nil {
				_, _ = writer.WriteString(fmt.Sprintf("ERR: %s\n", err.Error()))
			} else {
				_, _ = writer.WriteString(fmt.Sprintf("VALUE: %s\n", string(val)))
			}
		case "PUT":
			if len(parts) < 3 {
				_, _ = writer.WriteString("ERR: PUT requires key and value arguments\n")
				_ = writer.Flush()
				continue
			}
			key := []byte(parts[1])
			val := []byte(parts[2])
			err := s.db.Put(key, val)
			if err != nil {
				_, _ = writer.WriteString(fmt.Sprintf("ERR: %s\n", err.Error()))
			} else {
				_, _ = writer.WriteString("OK\n")
			}
		case "EXIT", "QUIT":
			_, _ = writer.WriteString("Goodbye!\n")
			_ = writer.Flush()
			return
		default:
			_, _ = writer.WriteString(fmt.Sprintf("ERR: Unknown command '%s'\n", cmd))
		}
		_ = writer.Flush()
	}
}
