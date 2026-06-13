package server

import (
	"bufio"
	"fmt"
	"net"
	"strings"

	"forgedb/internal/storage"
	"forgedb/internal/storage/btree"
)

// Server wraps a TCP listener and a KV database handle.
type Server struct {
	db       *storage.KV
	listener net.Listener
}

// NewServer creates a TCP server bound to addr, backed by the given KV store.
func NewServer(addr string, db *storage.KV) (*Server, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to bind to address %s: %w", addr, err)
	}
	return &Server{
		db:       db,
		listener: listener,
	}, nil
}

// Start accepts connections in a loop. Blocks until the listener is closed.
func (s *Server) Start() error {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return err
		}
		go s.handleConnection(conn)
	}
}

// Close shuts down the TCP listener.
func (s *Server) Close() error {
	return s.listener.Close()
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

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
			val, ok := s.db.Get([]byte(parts[1]))
			if !ok {
				_, _ = writer.WriteString("ERR: key not found\n")
			} else {
				_, _ = writer.WriteString(fmt.Sprintf("VALUE: %s\n", val))
			}

		case "PUT", "SET":
			if len(parts) < 3 {
				_, _ = writer.WriteString("ERR: PUT/SET requires a key and a value\n")
				_ = writer.Flush()
				continue
			}
			err := s.db.Set(&btree.InsertReq{
				Key:  []byte(parts[1]),
				Val:  []byte(parts[2]),
				Mode: btree.MODE_UPSERT,
			})
			if err != nil {
				_, _ = writer.WriteString(fmt.Sprintf("ERR: %s\n", err))
			} else {
				_, _ = writer.WriteString("OK\n")
			}

		case "DELETE", "DEL":
			if len(parts) < 2 {
				_, _ = writer.WriteString("ERR: DEL requires a key\n")
				_ = writer.Flush()
				continue
			}
			ok, err := s.db.Del(&btree.DeleteReq{
				Key: []byte(parts[1]),
			})
			if err != nil {
				_, _ = writer.WriteString(fmt.Sprintf("ERR: %s\n", err))
			} else if ok {
				_, _ = writer.WriteString("OK\n")
			} else {
				_, _ = writer.WriteString("ERR: key not found\n")
			}

		case "EXIT", "QUIT":
			_, _ = writer.WriteString("Goodbye!\n")
			_ = writer.Flush()
			return

		default:
			_, _ = writer.WriteString(fmt.Sprintf("ERR: unknown command '%s'\n", cmd))
		}

		_ = writer.Flush()
	}
}
