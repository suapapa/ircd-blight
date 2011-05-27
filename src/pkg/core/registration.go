package core

import (
	"kevlar/ircd/channel"
	"kevlar/ircd/log"
	"kevlar/ircd/parser"
	"kevlar/ircd/server"
	"kevlar/ircd/user"
	"os"
)

var (
	reghooks = []*Hook{
		Register(parser.CMD_NICK, Registration, AnyArgs, ConnReg),
		Register(parser.CMD_USER, Registration, AnyArgs, ConnReg),
		Register(parser.CMD_SERVER, Registration, AnyArgs, ConnReg),
		Register(parser.CMD_PASS, Registration, AnyArgs, ConnReg),
		Register(parser.CMD_CAPAB, Registration, AnyArgs, ConnReg),
	}
	quithook = Register(parser.CMD_QUIT, Any, AnyArgs, Quit)
)

// Handle the NICK, USER, SERVER, and PASS messages
func ConnReg(hook string, msg *parser.Message, ircd *IRCd) {
	var err os.Error
	var u *user.User
	var s *server.Server

	switch len(msg.SenderID) {
	case 3:
		s = server.Get(msg.SenderID)
	case 9:
		u = user.Get(msg.SenderID)
	}

	switch msg.Command {
	case parser.CMD_NICK:
		// NICK <nick>
		if u != nil {
			nick := msg.Args[0]
			err = u.SetNick(nick)
		}
	case parser.CMD_USER:
		// USER <user> . . :<real name>
		if u != nil {
			username, realname := msg.Args[0], msg.Args[3]
			err = u.SetUser(username, realname)
		}
	case parser.CMD_PASS:
		if s != nil {
			if len(msg.Args) != 4 {
				return
			}
			// PASS <password> TS <ver> <pfx>
			err = s.SetPass(msg.Args[0], msg.Args[2], msg.Args[3])
		}
	case parser.CMD_CAPAB:
		if s != nil {
			err = s.SetCapab(msg.Args[0])
		}
	case parser.CMD_SERVER:
		if s != nil {
			err = s.SetServer(msg.Args[0], msg.Args[1])
		}
	default:
		log.Warn.Printf("Unknown command %q", msg)
	}

	if u != nil {
		if err != nil {
			switch err := err.(type) {
			case *parser.Numeric:
				msg := err.Message()
				msg.DestIDs = append(msg.DestIDs, u.ID())
				ircd.ToClient <- msg
				return
			default:
				msg := &parser.Message{
					Command: parser.CMD_ERROR,
					Args:    []string{err.String()},
					DestIDs: []string{u.ID()},
				}
				ircd.ToClient <- msg
				return
			}
		}

		nickname, username, _, _ := u.Info()
		if nickname != "*" && username != "" {
			sendSignon(u, ircd)
			return
		}
	}

	if s != nil {
		if err != nil {
			switch err := err.(type) {
			case *parser.Numeric:
				msg := err.Message()
				msg.DestIDs = append(msg.DestIDs, s.ID())
				ircd.ToServer <- msg
				return
			default:
				msg := &parser.Message{
					Command: parser.CMD_ERROR,
					Args:    []string{err.String()},
					DestIDs: []string{s.ID()},
				}
				ircd.ToServer <- msg
				return
			}
		}

		sid, serv, pass, capab := s.Info()
		if sid != "" && serv != "" && pass != "" && len(capab) > 0 {
			sendServerSignon(s, ircd)
			Burst(s, ircd)
		}
	}
}

func sendSignon(u *user.User, ircd *IRCd) {
	log.Info.Printf("[%s] ** Registered\n", u.ID())
	u.SetType(user.RegisteredAsUser)

	destIDs := []string{u.ID()}
	// RPL_WELCOME
	msg := parser.NewNumeric(parser.RPL_WELCOME).Message()
	msg.Args[1] = "Welcome to the " + Config.Network.Name + " network, " + u.Nick() + "!"
	msg.DestIDs = destIDs
	ircd.ToClient <- msg

	// RPL_YOURHOST
	msg = parser.NewNumeric(parser.RPL_YOURHOST).Message()
	msg.Args[1] = "Your host is " + Config.Name + ", running IRCD-Blight" // TODO(kevlar): Version
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
	msg = parser.NewNumeric(parser.ERR_NOMOTD).Message()
	msg.DestIDs = destIDs
	ircd.ToClient <- msg

	msg = &parser.Message{
		Command: parser.CMD_MODE,
		Prefix:  "*",
		Args: []string{
			"*",
			"+i",
		},
		DestIDs: destIDs,
	}
	ircd.ToClient <- msg
}

func sendServerSignon(s *server.Server, ircd *IRCd) {
	log.Info.Printf("[%s] ** Registered As Server\n", s.ID())
	s.SetType(server.RegisteredAsServer)

	destIDs := []string{s.ID()}

	var msg *parser.Message

	msg = &parser.Message{
		Command: parser.CMD_PASS,
		Args: []string{
			"testpass", // TODO
			"TS",
			"6",
			Config.SID,
		},
		DestIDs: destIDs,
	}
	ircd.ToServer <- msg

	msg = &parser.Message{
		Command: parser.CMD_CAPAB,
		Args: []string{
			"QS ENCAP", // TODO
		},
		DestIDs: destIDs,
	}
	ircd.ToServer <- msg

	msg = &parser.Message{
		Command: parser.CMD_SERVER,
		Args: []string{
			Config.Name,
			"1",
			"IRCd",
		},
		DestIDs: destIDs,
	}
	ircd.ToServer <- msg
}

func Burst(serv *server.Server, ircd *IRCd) {
	destIDs := []string{serv.ID()}
	sid := Config.SID
	var msg *parser.Message

	// SID/SERVER
	// UID/EUID
	for uid := range user.Iter() {
		u := user.Get(uid)
		nick, username, name, typ := u.Info()
		if typ != user.RegisteredAsUser {
			continue
		}
		msg = &parser.Message{
			Prefix:  sid,
			Command: parser.CMD_UID,
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
	for channame := range channel.Iter() {
		chanobj, _ := channel.Get(channame, false)
		msg = &parser.Message{
			Prefix:  sid,
			Command: parser.CMD_SJOIN,
			Args: []string{
				chanobj.TS(),
				channame,
				// modes, params...
				"+nt",
			},
			DestIDs: destIDs,
		}
		for _, uid := range chanobj.UserIDs() {
			upfx := ""
			msg.Args = append(msg.Args, upfx+uid)
		}
		ircd.ToServer <- msg
	}
	// Optional: BMAST
	// Optional: TB
}

func Quit(hook string, msg *parser.Message, ircd *IRCd) {
	reason := "Client Quit"
	if len(msg.Args) > 0 {
		reason = msg.Args[0]
	}
	error := &parser.Message{
		Command: parser.CMD_ERROR,
		Args: []string{
			"Closing Link (" + reason + ")",
		},
		DestIDs: []string{
			msg.SenderID,
		},
	}
	ircd.ToClient <- error
}