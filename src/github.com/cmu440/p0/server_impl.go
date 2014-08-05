// Implementation of a MultiEchoServer. Students should write their code in this file.

package p0

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
)

type CtrlChan chan int
type ConnChan chan net.Conn
type MsgChan chan string
type ClientMap map[net.Conn]*Client

type Client struct {
	conn     net.Conn
	inChan   MsgChan
	outChan  MsgChan
	quitCtrl CtrlChan
}

type multiEchoServer struct {
	listener  net.Listener
	quitCtrl  CtrlChan
	addClient ConnChan
	delClient ConnChan
	broadcast MsgChan
	clients   ClientMap
}

// New creates and returns (but does not start) a new MultiEchoServer.
func New() MultiEchoServer {
	mes := new(multiEchoServer)
	mes.quitCtrl = make(CtrlChan)
	mes.addClient = make(ConnChan)
	mes.delClient = make(ConnChan)
	mes.broadcast = make(MsgChan)
	mes.clients = make(ClientMap)

	return mes
}

func (mes *multiEchoServer) Start(port int) error {
	errChan := make(chan error)
	go func() {
		fmt.Printf("Starting server\n")
		listener, err := net.Listen("tcp", ":"+strconv.Itoa(port))
		errChan <- err
		if err != nil {
			fmt.Printf("Cannot start listening: %s\n", err.Error())
			return
		}
		fmt.Printf("Server is listening on port %d\n", port)
		mes.listener = listener
		go control(mes)
		for {
			fmt.Printf("Accepting connection...\n")
			conn, err := mes.listener.Accept()
			if err != nil {
				fmt.Printf("Cannot accept: %s\n", err.Error())
				return
			}
			fmt.Printf("Accepted connection from %s\n", conn.RemoteAddr().String())
			mes.addClient <- conn
		}
	}()

	return <-errChan
}

func (mes *multiEchoServer) Close() {
	mes.quitCtrl <- -1

	return
}

func (mes *multiEchoServer) Count() int {
	return len(mes.clients)
}

func (mes *multiEchoServer) join(conn net.Conn) {
	if conn == nil {
		return
	}
	client := newClient(conn)

	if _, dup := mes.clients[conn]; !dup {
		mes.clients[conn] = &client
	}

	go clientRoutine(&client, mes)
}

func (mes *multiEchoServer) remove(conn net.Conn) {
	conn.Close()
	if _, exists := mes.clients[conn]; exists {
		delete(mes.clients, conn)
	}
}

func (mes *multiEchoServer) broadcastMsg(message string) {
	go func() {
		for conn, _ := range mes.clients {
			select {
			case mes.clients[conn].outChan <- message:
				fmt.Printf("%s <- %s", conn.RemoteAddr().String(), message)
			default:
				fmt.Printf("%s buffer full, drop message: %s", conn.RemoteAddr().String(), message)
			}
		}
	}()
}

func (mes *multiEchoServer) quit() {
	fmt.Printf("server quitting\n")
	for conn, _ := range mes.clients {
		mes.clients[conn].quitCtrl <- -1
	}
	mes.listener.Close()
}

func control(mes *multiEchoServer) {
	for {
		select {
		case conn := <-mes.addClient:
			fmt.Printf("adding client %s\n", conn.RemoteAddr().String())
			mes.join(conn)
		case conn := <-mes.delClient:
			fmt.Printf("deleting client %s\n", conn.RemoteAddr().String())
			mes.remove(conn)
		case message := <-mes.broadcast:
			fmt.Printf("broadcasting %s", message)
			mes.broadcastMsg(message)
		case <-mes.quitCtrl:
			fmt.Printf("Server quitting\n")
			mes.quit()
			return
		}
	}
}

func newClient(conn net.Conn) Client {
	client := new(Client)
	client.conn = conn
	client.inChan = make(MsgChan)
	client.outChan = make(MsgChan, 100)
	client.quitCtrl = make(CtrlChan)

	go client.Reader()
	go client.Writer()

	return *client
}

func (client *Client) Reader() {
	reader := bufio.NewReader(client.conn)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			fmt.Printf("%s received EOF, closing\n", client.conn.RemoteAddr().String())
			client.quitCtrl <- -1
			return
		}
		fmt.Printf("%s: received %s", client.conn.RemoteAddr().String(), string(line))
		client.inChan <- string(line)
	}
}

func (client *Client) Writer() {
	for message := range client.outChan {
		fmt.Printf("%s: writing %s", client.conn.RemoteAddr().String(), message)
		client.conn.Write([]byte(message))
		//return?
	}
}

func clientRoutine(client *Client, mes *multiEchoServer) {
	for {
		select {
		case message := <-client.inChan:
			mes.broadcast <- message
		case <-client.quitCtrl:
			mes.delClient <- client.conn
			return
		}
	}
}
