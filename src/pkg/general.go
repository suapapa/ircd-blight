package ircd

var (
	pinghooks = []*Hook{
		Register(CMD_PING, EMASK_USER, NArgs(1), Ping),
		Register(CMD_PING, EMASK_SERVER, OptArgs(1, 1), SPing),
		Register(CMD_PONG, EMASK_SERVER, OptArgs(1, 1), SPing),
	}
)

func Ping(hook string, msg *Message, ircd *IRCd) {
	pongmsg := msg.Args[0]
	ircd.ToClient <- &Message{
		Command: CMD_PONG,
		Args: []string{
			Config.Name,
			pongmsg,
		},
		DestIDs: []string{
			msg.SenderID,
		},
	}
}

func SPing(hook string, msg *Message, ircd *IRCd) {
	source := msg.Args[0]
	dest := Config.SID
	if len(msg.Args) > 1 {
		dest = msg.Args[1]
	}

	if dest == Config.SID {
		switch hook {
		case CMD_PING:
			ircd.ToServer <- &Message{
				Prefix:  Config.SID,
				Command: CMD_PONG,
				Args: []string{
					Config.Name,
					source,
				},
				DestIDs: []string{
					msg.SenderID,
				},
			}
		case CMD_PONG:
			Info.Printf("End of BURST from %s", source)
		}
	} else {
		for sid := range IterFor([]string{dest}, "") {
			Debug.Printf("Forwarding %s to %s", hook, sid)
			msg := msg.Dup()
			msg.DestIDs = []string{sid}
			ircd.ToServer <- msg
		}
	}
}
