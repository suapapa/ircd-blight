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

func (s *IRCd) manageServers() {
	defer s.running.Done()

	sid2conn := make(map[string]*Conn)
	upstream := ""
	_ = upstream

	var open bool = true
	var msg *Message
	for open {
		// Check if we are connected to our upstream
		// TODO(kevlar): upstream

		select {
		//// Incoming and outgoing messages to and from servers
		// Messages directly from connections
		case msg = <-s.fromServer:
			var conn *Conn
			var ok bool
			if conn, ok = sid2conn[msg.SenderID]; !ok {
				Debug.Printf("{%s} >> [DROPPING] %s\n", msg.SenderID, msg)
				continue
			}
			Debug.Printf("{%s} >> %s\n", msg.SenderID, msg)

			if msg.Command == CMD_ERROR {
				if conn != nil {
					Debug.Printf("{%s} ** Connection terminated remotely", msg.SenderID)
					sid2conn[msg.SenderID] = nil
					conn.UnsubscribeClose(s.serverClosing)
					conn.Close()
					if IsLocal(msg.SenderID) {
						DispatchServer(&Message{
							SenderID: msg.SenderID,
							Command:  CMD_SQUIT,
							Args: []string{
								msg.SenderID,
								"Unexpected ERROR on connection",
							},
						}, s)
					}
				}
				continue
			}

			DispatchServer(msg, s)

		// Messages from hooks
		case msg, open = <-s.ToServer:
			// Count the number of messages sent
			sentcount := 0
			for _, dest := range msg.DestIDs {
				Debug.Printf("{%v} << %s\n", dest, msg)

				if conn, ok := sid2conn[dest]; ok {
					conn.WriteMessage(msg)
					sentcount++
				} else {
					Warn.Printf("Unknown SID %s", dest)
				}
			}

			if sentcount == 0 {
				Warn.Printf("Dropped outgoing server message: %s", msg)
			}

		//// Connection management
		// Connecting servers
		case conn := <-s.newServer:
			sid := conn.ID()
			sid2conn[sid] = conn
			GetServer(sid, true)
			Debug.Printf("{%s} ** Registered connection", sid)
			conn.Subscribe(s.fromServer)
			conn.SubscribeClose(s.serverClosing)
		// Disconnecting servers
		case closeid := <-s.serverClosing:
			Debug.Printf("{%s} ** Connection closed", closeid)
			sid2conn[closeid] = nil
			if IsLocal(closeid) {
				DispatchServer(&Message{
					SenderID: closeid,
					Command:  CMD_SQUIT,
					Args: []string{
						closeid,
						"Connection close",
					},
				}, s)
			}
		}
	}
}

func (s *IRCd) manageClients() {
	defer s.running.Done()

	uid2conn := make(map[string]*Conn)

	var open bool = true
	var msg *Message
	for open {
		// anything to do here?

		select {
		//// Incoming and outgoing messages to and from clients
		// Messages directly from connections
		//   TODO(kevlar): Collapse into one case (I don't like that server gets split)
		case msg = <-s.fromClient:
			u := GetUser(msg.SenderID)
			uid := u.ID()

			if msg.Command == CMD_ERROR {
				Delete(uid)
				conn := uid2conn[uid]
				if conn != nil {
					Debug.Printf("[%s] ** Connection terminated remotely", uid)
					Delete(uid)
					uid2conn[uid] = nil
					conn.UnsubscribeClose(s.clientClosing)
					conn.Close()
				}
				continue
			}

			Debug.Printf("[%s] >> %s", uid, msg)
			DispatchClient(msg, s)

		// Messages from hooks
		case msg, open = <-s.ToClient:
			// Handle internal messages
			switch msg.Command {
			case INT_DELUSER:
				// This is sent over the channel to ensure that all user messages
				// which may need to query these UIDs are sent before they are
				// removed.
				for _, uid := range msg.DestIDs {
					Debug.Printf("[%s] Netsplit", uid)
					Delete(uid)
				}
				continue
			}

			// Count the number of messages sent
			sentcount := 0

			// For simplicity, * in the prefix or as the first argument
			// is replaced by the nick of the user the message is sent to
			setnick := len(msg.Args) > 0 && msg.Args[0] == "*"
			setprefix := msg.Prefix == "*"

			closeafter := false
			if msg.Command == CMD_ERROR {
				// Close the connection and remove the prefix if we are sending an ERROR
				closeafter = true
				msg.Prefix = ""
			} else if len(msg.Prefix) == 0 {
				// Make sure a prefix is specified (use the server name)
				msg.Prefix = Config.Name
			}

			local := make([]string, 0, len(msg.DestIDs))
			remote := make([]string, 0, len(msg.DestIDs))

			for _, id := range msg.DestIDs {
				if id[:3] != Config.SID {
					remote = append(remote, id)
					continue
				}
				local = append(local, id)
			}

			// Pass the message to the server goroutine
			if len(remote) > 0 {
				if closeafter {
					for _, id := range remote {
						Delete(id)
					}
				} else {
					Warn.Printf("Dropping non-local: %s", len(remote), msg)
				}

				// Short circuit if there are no local recipients
				if len(local) == 0 {
					continue
				}
			}

			// Examine all arguments for UIDs and replace them
			if isuid(msg.Prefix) {
				nick, user, _, _, ok := GetUserInfo(msg.Prefix)
				if !ok {
					Warn.Printf("Nonexistent ID %s as prefix", msg.Prefix)
				} else {
					msg.Prefix = nick + "!" + user + "@host" // TODO(kevlar): hostname
				}
			}
			for i := range msg.Args {
				if isuid(msg.Args[i]) {
					nick, _, _, _, ok := GetUserInfo(msg.Args[i])
					if !ok {
						Warn.Printf("Nonexistent ID %s as argument", msg.Args[i])
						continue
					}
					msg.Args[i] = nick
				}
			}

			for _, id := range msg.DestIDs {
				conn, ok := uid2conn[id]
				if !ok {
					Warn.Printf("Nonexistent ID %s in send", id)
					continue
				}
				if setnick || setprefix {
					nick, _, _, _, _ := GetUserInfo(id)
					if setnick {
						msg.Args[0] = nick
					}
					if setprefix {
						msg.Prefix = nick
					}
				}
				conn.WriteMessage(msg)
				Debug.Printf("[%s] << %s\n", id, msg)
				sentcount++
				if closeafter {
					Debug.Printf("[%s] ** Connection terminated", id)
					Delete(id)
					uid2conn[id] = nil
					conn.UnsubscribeClose(s.clientClosing)
					conn.Close()
				}
			}
			if sentcount == 0 {
				Warn.Printf("Dropped outgoing client message: %s", msg)
			}

		// Connecting clients
		case conn := <-s.newClient:
			id := conn.ID()
			uid2conn[id] = conn
			GetUser(id)
			conn.Subscribe(s.fromClient)
			conn.SubscribeClose(s.clientClosing)
		// Disconnecting clients
		case closeid := <-s.clientClosing:
			Debug.Printf("[%s] ** Connection closed", closeid)
			Delete(closeid)
			uid2conn[closeid] = nil
		}
	}
}

func (s *IRCd) manageIncoming() {
	defer s.running.Done()

	quit := false
	defer func() {
		quit = true
	}()

	manage := func(conn *Conn) {
		inc := make(chan *Message)
		stop := make(chan string)
		conn.Subscribe(inc)
		conn.SubscribeClose(stop)

		user, nick := false, false
		pass, server, capab := false, false, false
		sid := ""

		queued := make([]*Message, 0, 3)

		for !quit {
			select {
			case msg := <-inc:
				Debug.Printf(" %s  %s", msg.SenderID, msg)
				queued = append(queued, msg)
				switch msg.Command {
				case CMD_PASS:
					if len(msg.Args) == 4 {
						pass = true
						sid = msg.Args[3]
					}
				case CMD_USER:
					user = true
				case CMD_NICK:
					nick = true
				case CMD_CAPAB:
					capab = true
				case CMD_SERVER:
					server = true
				}
			case <-stop:
				return
			}

			if !quit && nick && user {
				conn.Unsubscribe(inc)
				conn.UnsubscribeClose(stop)
				s.newClient <- conn
				for _, msg := range queued {
					s.fromClient <- msg
				}
				return
			}
			if !quit && pass && server && capab {
				conn.SetServer(sid)
				conn.Unsubscribe(inc)
				conn.UnsubscribeClose(stop)
				s.newServer <- conn
				for _, msg := range queued {
					msg.SenderID = sid
					s.fromServer <- msg
				}
				return
			}
		}
	}

	for {
		select {
		// Connecting clients
		case conn := <-s.Incoming:
			go manage(conn)
		}
	}
}

var (
	// TODO(kevlar): Configurable?
	SendQ = 100
	RecvQ = 100
)

func (s *IRCd) Quit() {
	close(s.Incoming)
	close(s.ToClient)
	close(s.ToServer)
	s.running.Wait()
}

func Start() {
	// Make sure the configuration is good before we do anything
	if !Config.Check() {
		Error.Fatalf("Could not start: invalid configuration")
	}

	listener := NewListener()
	defer listener.Close()
	for _, ports := range Config.Ports {
		portlist, err := ports.GetPortList()
		if err != nil {
			Warn.Print(err)
		}
		for _, port := range portlist {
			listener.AddPort(port)
		}
	}

	s := &IRCd{
		Incoming:      listener.Incoming,
		newClient:     make(chan *Conn),
		newServer:     make(chan *Conn),
		clientClosing: make(chan string),
		serverClosing: make(chan string),

		ToClient:   make(chan *Message, SendQ),
		ToServer:   make(chan *Message, SendQ),
		fromClient: make(chan *Message, SendQ),
		fromServer: make(chan *Message, SendQ),

		running: new(sync.WaitGroup),
	}

	s.running.Add(1)
	go s.manageClients()

	s.running.Add(1)
	go s.manageServers()

	s.running.Add(1)
	go s.manageIncoming()

	s.running.Wait()
}

func isuid(id string) bool {
	return len(id) == 9 && id[0] >= '0' && id[0] <= '9'
}
