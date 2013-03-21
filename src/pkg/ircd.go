package ircd

import (
	"sync"
)

type IRCd struct {
	Incoming      chan *Conn
	newClient     chan *Conn
	newServer     chan *Conn
	clientClosing chan string
	serverClosing chan string

	ToClient   chan *Message
	ToServer   chan *Message
	fromClient chan *Message
	fromServer chan *Message

	running *sync.WaitGroup
}
