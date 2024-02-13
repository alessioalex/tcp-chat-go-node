package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	helpCmd = "HELP"
	listCmd = "LIST"
	sayCmd  = "SAY"
	exitCmd = "EXIT"
)

var (
	helpMsg = []byte("Available commands: HELP, LIST, EXIT, SAY.")
)

type BcastMsg struct {
	by      string
	value   string
	exclude net.Conn
}

func main() {
	getNickname, err := createNicknameGenerator("./nicknames.json")

	if err != nil {
		panic(err)
	}

	// Listen for incoming connections
	listener, err := net.Listen("tcp", "localhost:9999")
	if err != nil {
		panic(err)
	}
	defer listener.Close()

	fmt.Println("Server is listening on port 9999")
	broadcastChan := make(chan *BcastMsg)
	defer close(broadcastChan)

	go broadcaster(broadcastChan)

	for {
		// Accept incoming connections
		conn, err := listener.Accept()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Incoming connection error:", err)
			continue
		}

		nickname := getNickname()

		// Handle client connection in a goroutine
		go handleClient(conn, nickname, broadcastChan)
	}
}

func sendToClient(conn net.Conn, clientPrefix string, msg string) {
	fmt.Printf("DBG: Sending %q to client %q\n", msg, clientPrefix)

	data := []byte(fmt.Sprintf(
		"%s %s >> %s\n",
		time.Now().Format("15:04:05"),
		clientPrefix,
		msg,
	))

	_, err := conn.Write(data)

	if err != nil {
		fmt.Fprintln(os.Stderr, "Error when writing to client:", err)
		return
	}
}

// "thread safe" (as Java people would call this) Map
var clients sync.Map

func handleClient(conn net.Conn, nickname string, broadcastChan chan<- *BcastMsg) {
	defer conn.Close()

	byServer := "[SERVER]"
	clientPrefix := fmt.Sprintf("%s@%s", nickname, conn.RemoteAddr().String())
	clients.Store(clientPrefix, conn)

	newUserJoinedMsg := fmt.Sprintf("%s has joined the club", clientPrefix)
	broadcastChan <- &BcastMsg{
		by:    byServer,
		value: newUserJoinedMsg,
		// no need to announce a client's presence to himself
		exclude: conn,
	}

	welcomeMsg := fmt.Sprintf("%s is your nickname, welcome!", clientPrefix)
	sendToClient(conn, byServer, welcomeMsg)
	fmt.Printf("%s %s\n", clientPrefix, welcomeMsg)

	// Create a buffer to read data into
	// 16*1024
	buffer := make([]byte, 16384)
	msgRe, _ := regexp.Compile(`^(HELP|EXIT|LIST|(SAY) (.+))(\r\n|\n|\r)?$`)

	for {
		// Read data from the client
		bytesRead, err := conn.Read(buffer)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error when reading from client:", err)
			conn.Close()
			clients.Delete(clientPrefix)

			broadcastChan <- &BcastMsg{
				by:    byServer,
				value: fmt.Sprintf("%s has left the building", clientPrefix),
			}

			return
		}

		buf := buffer[:bytesRead-1]
		fmt.Printf("Received: %q\n", buf)

		matches := msgRe.FindStringSubmatch(string(buf))
		fmt.Printf("%#v\n", matches)

		cmd := helpCmd
		var whatToSay string
		if len(matches) != 0 && matches[2] == sayCmd {
			cmd = sayCmd
			whatToSay = matches[3]
		} else if len(matches) != 0 {
			cmd = matches[1]
		}

		executeCommand(conn, broadcastChan, clientPrefix, cmd, whatToSay)
	}
}

func executeCommand(
	conn net.Conn, broadcastChan chan<- *BcastMsg,
	clientPrefix string, cmd string, whatToSay string,
) {
	fmt.Printf("DBG: Executing command %s\n", cmd)
	byServer := "[SERVER]"

	switch cmd {
	case listCmd:
		var msg strings.Builder
		count := 0
		delimiter := ", "

		clients.Range(func(key, value interface{}) bool {
			if count != 0 {
				msg.WriteString(delimiter)
			}
			msg.WriteString(key.(string))

			count++
			return true
		})

		fullMsg := fmt.Sprintf("%d clients connected: %s.", count, msg.String())
		sendToClient(conn, byServer, fullMsg)
	case exitCmd:
		sendToClient(conn, byServer, "bye")
		conn.Close()
	case sayCmd:
		broadcastChan <- &BcastMsg{
			by:    clientPrefix,
			value: whatToSay,
		}
	// HELP case
	default:
		sendToClient(conn, byServer, "Available commands: HELP, LIST, EXIT, SAY.")
	}
}

func broadcaster(broadcastChan <-chan *BcastMsg) {
	for {
		msg := <-broadcastChan
		fmt.Printf("[Broadcaster] -> %s\n", msg)

		clients.Range(func(_, value interface{}) bool {
			conn, ok := value.(net.Conn)

			if ok && msg.exclude != conn {
				sendToClient(conn, msg.by, msg.value)
			}

			return true
		})
	}
}

func createNicknameGenerator(filePath string) (func() string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("error reading file %q: %w", filePath, err)
	}

	var availableNicknames []string
	if err = json.Unmarshal(content, &availableNicknames); err != nil {
		return nil, fmt.Errorf("error parsing JSON file %q: %w", filePath, err)
	}

	// Initialize global pseudo random generator
	rand.Seed(time.Now().UnixNano())

	return func() string {
		return availableNicknames[rand.Intn(len(availableNicknames))]
	}, nil
}
