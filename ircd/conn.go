package ircd

import (
	"bufio"
	"log"
	"net"
)

type Conn struct {
	net.Conn
	active      bool
	subscribers map[chan<- *Message]bool
	onclose     map[chan<- string]bool
	Error       error
	id          string
	reading     bool
}

func NewConn(nc net.Conn) *Conn {
	c := &Conn{
		Conn:        nc,
		active:      true,
		subscribers: make(map[chan<- *Message]bool),
		onclose:     make(map[chan<- string]bool),
		id:          NextUserID(),
	}
	log.Printf("[%s] ** Connected", c.id)
	return c
}

func (c *Conn) Close() error {
	for ch := range c.onclose {
		ch <- c.id
	}
	return c.Conn.Close()
}

func (c *Conn) ID() string {
	return c.id
}

func (c *Conn) readthread() {
	// Always close the connection
	defer c.Close()

	// Read lines by \r\n or \n
	linereader := bufio.NewReader(c)
	for c.active {
		line, _, err := linereader.ReadLine()
		if err != nil {
			c.active = false
			c.Error = err
			return
		}
		message := ParseMessage(line)
		message.SenderID = c.id
		if message != nil {
			for subscriber := range c.subscribers {
				subscriber <- message
			}
		}
	}
}

func (c *Conn) WriteMessage(message *Message) {
	bytes := message.Bytes()
	bytes = append(bytes, '\r', '\n')
	n, err := c.Write(bytes)
	if err != nil || n != len(bytes) {
		c.Error = err
		c.active = false
		c.Close()
	}
}

func (c *Conn) Active() bool {
	return c.active
}

func (c *Conn) Subscribe(chn chan<- *Message) {
	c.subscribers[chn] = true

	if !c.reading {
		go c.readthread()
		c.reading = true
	}
}

func (c *Conn) SubscribeClose(chn chan<- string) {
	c.onclose[chn] = true
}

func (c *Conn) Unsubscribe(chn chan *Message) {
	delete(c.subscribers, chn)
}

func (c *Conn) UnsubscribeClose(chn chan<- string) {
	delete(c.onclose, chn)
}

func (c *Conn) SetServer(id string) {
	if len(c.id) != 9 {
		panic("SetServer on invalid connection")
	}
	c.id = id
}
