package ircd

import (
	"strings"
)

var (
	reghooks = []*Hook{
		Register(CMD_NICK, EMASK_REGISTRATION, AnyArgs, ConnReg),
		Register(CMD_USER, EMASK_REGISTRATION, AnyArgs, ConnReg),
		Register(CMD_SERVER, EMASK_REGISTRATION, AnyArgs, ConnReg),
		Register(CMD_PASS, EMASK_REGISTRATION, AnyArgs, ConnReg),
		Register(CMD_CAPAB, EMASK_REGISTRATION, AnyArgs, ConnReg),
		Register(CMD_UID, EMASK_SERVER, NArgs(9), Uid),
		Register(CMD_SID, EMASK_SERVER, NArgs(4), Sid),
	}
	quithooks = []*Hook{
		Register(CMD_QUIT, EMASK_USER, AnyArgs, Quit),
		Register(CMD_QUIT, EMASK_SERVER, NArgs(2), Quit),
		Register(CMD_SQUIT, EMASK_SERVER, NArgs(2), SQuit),
	}
)

// Handle the NICK, USER, SERVER, and PASS messages
func ConnReg(hook string, msg *Message, ircd *IRCd) {
	var err error
	var u *User
	var s *Server

	switch len(msg.SenderID) {
	case 3:
		s = GetServer(msg.SenderID, true)
	case 9:
		u = GetUser(msg.SenderID)
	}

	switch msg.Command {
	case CMD_NICK:
		// NICK <nick>
		if u != nil {
			nick := msg.Args[0]
			err = u.SetNick(nick)
		}
	case CMD_USER:
		// USER <user> . . :<real name>
		if u != nil {
			username, realname := msg.Args[0], msg.Args[3]
			err = u.SetUser(username, realname)
		}
	case CMD_PASS:
		if s != nil {
			if len(msg.Args) != 4 {
				return
			}
			// PASS <password> TS <ver> <pfx>
			err = s.SetPass(msg.Args[0], msg.Args[2], msg.Args[3])
		}
	case CMD_CAPAB:
		if s != nil {
			err = s.SetCapab(msg.Args[0])
		}
	case CMD_SERVER:
		if s != nil {
			err = s.SetServer(msg.Args[0], msg.Args[1])
		}
	default:
		Warn.Printf("Unknown command %q", msg)
	}

	if u != nil {
		if err != nil {
			switch err := err.(type) {
			case *Numeric:
				msg := err.Message()
				msg.DestIDs = append(msg.DestIDs, u.ID())
				ircd.ToClient <- msg
				return
			default:
				msg := &Message{
					Command: CMD_ERROR,
					Args:    []string{err.Error()},
					DestIDs: []string{u.ID()},
				}
				ircd.ToClient <- msg
				return
			}
		}

		nickname, username, realname, _ := u.Info()
		if nickname != "*" && username != "" {
			// Notify servers
			for sid := range ServerIter() {
				ircd.ToServer <- &Message{
					Prefix:  Config.SID,
					Command: CMD_UID,
					Args: []string{
						nickname,
						"1",
						u.TS(),
						"+i",
						username,
						"some.host",
						"127.0.0.1",
						u.ID(),
						realname,
					},
					DestIDs: []string{sid},
				}
			}

			// Process signon
			sendSignon(u, ircd)
			return
		}
	}

	if s != nil {
		if err != nil {
			switch err := err.(type) {
			case *Numeric:
				msg := err.Message()
				msg.DestIDs = append(msg.DestIDs, s.ID())
				ircd.ToServer <- msg
				return
			default:
				msg := &Message{
					Command: CMD_ERROR,
					Args:    []string{err.Error()},
					DestIDs: []string{s.ID()},
				}
				ircd.ToServer <- msg
				return
			}
		}

		sid, serv, pass, capab := s.Info()
		if sid != "" && serv != "" && pass != "" && len(capab) > 0 {
			// Notify servers
			for sid := range ServerIter() {
				ircd.ToServer <- &Message{
					Prefix:  Config.SID,
					Command: CMD_SID,
					Args: []string{
						serv,
						"2",
						sid,
						"some server",
					},
					DestIDs: []string{sid},
				}
			}

			sendServerSignon(s, ircd)
			Burst(s, ircd)
		}
	}
}

func sendSignon(u *User, ircd *IRCd) {
	Info.Printf("[%s] ** Registered\n", u.ID())
	u.SetType(RegisteredAsUser)

	destIDs := []string{u.ID()}
	// RPL_WELCOME
	msg := NewNumeric(RPL_WELCOME).Message()
	msg.Args[1] = "Welcome to the " + Config.Network.Name + " network, " + u.Nick() + "!"
	msg.DestIDs = destIDs
	ircd.ToClient <- msg

	// RPL_YOURHOST
	msg = NewNumeric(RPL_YOURHOST).Message()
	msg.Args[1] = "Your host is " + Config.Name + ", running " + REPO_VERSION
	msg.DestIDs = destIDs
	ircd.ToClient <- msg

	// RPL_CREATED
	// RPL_MYINFO
	// RPL_ISUPPORT

	// RPL_LUSERCLIENT
	// RPL_LUSEROP
	// RPL_LUSERUNKNOWN
	// RPL_LUSERCHANNELS
	// RPL_LUSERME

	// RPL_LOCALUSERS
	// RPL_GLOBALUSERS

	// RPL_NOMOTD
	msg = NewNumeric(ERR_NOMOTD).Message()
	msg.DestIDs = destIDs
	ircd.ToClient <- msg

	msg = &Message{
		Command: CMD_MODE,
		Prefix:  "*",
		Args: []string{
			"*",
			"+i",
		},
		DestIDs: destIDs,
	}
	ircd.ToClient <- msg
}

func sendServerSignon(s *Server, ircd *IRCd) {
	Info.Printf("{%s} ** Registered As Server\n", s.ID())
	s.SetType(RegisteredAsServer)

	destIDs := []string{s.ID()}

	var msg *Message

	msg = &Message{
		Command: CMD_PASS,
		Args: []string{
			"testpass", // TODO
			"TS",
			"6",
			Config.SID,
		},
		DestIDs: destIDs,
	}
	ircd.ToServer <- msg

	msg = &Message{
		Command: CMD_CAPAB,
		Args: []string{
			//"QS EX CHW IE KLN KNOCK TB UNKLN CLUSTER ENCAP SERVICES RSFNC SAVE EUID EOPMOD BAN MLOCK",
			"QS ENCAP", // TODO
		},
		DestIDs: destIDs,
	}
	ircd.ToServer <- msg

	msg = &Message{
		Command: CMD_SERVER,
		Args: []string{
			Config.Name,
			"1",
			"IRCd",
		},
		DestIDs: destIDs,
	}
	ircd.ToServer <- msg
}

func Burst(serv *Server, ircd *IRCd) {
	destIDs := []string{serv.ID()}
	sid := Config.SID
	var msg *Message

	// SID/SERVER
	// UID/EUID
	for uid := range UserIter() {
		u := GetUser(uid)
		nick, username, name, typ := u.Info()
		if typ != RegisteredAsUser {
			continue
		}
		msg = &Message{
			Prefix:  sid,
			Command: CMD_UID,
			Args: []string{
				nick,
				// hopcount
				"1",
				u.TS(),
				// umodes
				"+i",
				username,
				// visible hostname
				"some.host",
				// IP addr
				"127.0.0.1",
				uid,
				name,
			},
			DestIDs: destIDs,
		}
		ircd.ToServer <- msg
	}
	// Optional: ENCAP REALHOST, ENCAP LOGIN, AWAY
	// SJOIN
	for channame := range ChannelIter() {
		chanobj, _ := GetChannel(channame, false)
		msg = &Message{
			Prefix:  sid,
			Command: CMD_SJOIN,
			Args: []string{
				chanobj.TS(),
				channame,
				// modes, params...
				"+", // "+nt",
				strings.Join(chanobj.UserIDsWithPrefix(), " "),
			},
			DestIDs: destIDs,
		}
		ircd.ToServer <- msg
	}
	// Optional: BMAST
	// Optional: TB
}

func Uid(hook string, msg *Message, ircd *IRCd) {
	nickname, hopcount, nickTS := msg.Args[0], msg.Args[1], msg.Args[2]
	umode, username, hostname := msg.Args[3], msg.Args[4], msg.Args[5]
	ip, uid, name := msg.Args[6], msg.Args[7], msg.Args[8]
	_ = umode

	err := Import(uid, nickname, username, hostname, ip, hopcount, nickTS, name)
	if err != nil {
		// TODO: TS check - Kill remote or local? For now, we kill remote.
		ircd.ToServer <- &Message{
			Prefix:  Config.SID,
			Command: CMD_SQUIT,
			Args: []string{
				uid,
				err.Error(),
			},
			DestIDs: []string{msg.SenderID},
		}
	}

	for fwd := range ServerIter() {
		if fwd != msg.SenderID {
			Debug.Printf("Forwarding UID from %s to %s", msg.SenderID, fwd)
			fmsg := msg.Dup()
			fmsg.DestIDs = []string{fwd}
		}
	}
}

func Sid(hook string, msg *Message, ircd *IRCd) {
	servname, hopcount, sid, desc := msg.Args[0], msg.Args[1], msg.Args[2], msg.Args[3]

	err := LinkServer(msg.Prefix, sid, servname, hopcount, desc)
	if err != nil {
		ircd.ToServer <- &Message{
			Prefix:  Config.SID,
			Command: CMD_SQUIT,
			Args: []string{
				sid,
				err.Error(),
			},
			DestIDs: []string{msg.SenderID},
		}
	}

	for fwd := range ServerIter() {
		if fwd != msg.SenderID {
			Debug.Printf("Forwarding SID from %s to %s", msg.SenderID, fwd)
			fmsg := msg.Dup()
			fmsg.DestIDs = []string{fwd}
		}
	}
}

func Quit(hook string, msg *Message, ircd *IRCd) {
	quitter := msg.SenderID
	reason := "Client Quit"

	if len(msg.Args) > 0 {
		reason = msg.Args[0]
	}

	if len(msg.SenderID) == 3 {
		quitter = msg.Prefix
	}

	for sid := range ServerIter() {
		Debug.Printf("Forwarding QUIT from %s to %s", quitter, sid)
		if sid != msg.SenderID {
			ircd.ToServer <- &Message{
				Prefix:  quitter,
				Command: CMD_QUIT,
				Args: []string{
					reason,
				},
				DestIDs: []string{sid},
			}
		}
	}

	members := PartAll(quitter)
	Debug.Printf("QUIT recipients: %#v", members)
	peers := make(map[string]bool)
	for _, users := range members {
		for _, uid := range users {
			if uid[:3] == Config.SID && uid != quitter {
				peers[uid] = true
			}
		}
	}
	if len(peers) > 0 {
		notify := []string{}
		for peer := range peers {
			notify = append(notify, peer)
		}
		ircd.ToClient <- &Message{
			Prefix:  quitter,
			Command: CMD_QUIT,
			Args: []string{
				"Quit: " + reason,
			},
			DestIDs: notify,
		}
	}

	// Will be dropped if it's a remote client
	error := &Message{
		Command: CMD_ERROR,
		Args: []string{
			"Closing Link (" + reason + ")",
		},
		DestIDs: []string{
			quitter,
		},
	}
	ircd.ToClient <- error
}

func SQuit(hook string, msg *Message, ircd *IRCd) {
	split, reason := msg.Args[0], msg.Args[1]

	if split == Config.SID {
		split = msg.SenderID
	}

	// Forward
	for sid := range ServerIter() {
		if sid != msg.SenderID {
			msg := msg.Dup()
			msg.DestIDs = []string{sid}
			ircd.ToServer <- msg
		}
	}
	if IsLocal(split) {
		ircd.ToServer <- &Message{
			Command: CMD_ERROR,
			Args: []string{
				"SQUIT: " + reason,
			},
		}
	}

	sids := Unlink(split)
	peers := UserSplit(sids)
	notify := ChanSplit(Config.SID, peers)

	Debug.Printf("NET SPLIT: %s", split)
	Debug.Printf(" -   SIDs: %v", sids)
	Debug.Printf(" -  Peers: %v", peers)
	Debug.Printf(" - Notify: %v", notify)

	for uid, peers := range notify {
		if len(peers) > 0 {
			ircd.ToClient <- &Message{
				Prefix:  uid,
				Command: CMD_QUIT,
				Args: []string{
					"*.net *.split",
				},
				DestIDs: peers,
			}
		}
	}
	// Delete all of the peers
	if len(peers) > 0 {
		ircd.ToClient <- &Message{
			Command: INT_DELUSER,
			DestIDs: peers,
		}
	}
}
