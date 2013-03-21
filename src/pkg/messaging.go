package ircd

import (
	"strings"
)

var (
	msghooks = []*Hook{
		Register(CMD_PRIVMSG, EMASK_USER|EMASK_SERVER, NArgs(2), Privmsg),
		Register(CMD_NOTICE, EMASK_USER|EMASK_SERVER, NArgs(2), Privmsg),
	}
)

func Privmsg(hook string, msg *Message, ircd *IRCd) {
	quiet := hook == CMD_NOTICE
	recipients, text := strings.Split(msg.Args[0], ","), msg.Args[1]
	sender := msg.SenderID
	if len(msg.Prefix) == 9 {
		sender = msg.Prefix
	}
	local := []string{}
	remote := []string{}
	for _, name := range recipients {
		if ValidChannel(name) {
			channel, err := GetChannel(name, false)
			if num, ok := err.(*Numeric); ok {
				if !quiet {
					ircd.ToClient <- num.Message(msg.SenderID)
				}
				continue
			}
			local := []string{}
			remote := []string{}
			for _, uid := range channel.UserIDs() {
				if uid != sender {
					if uid[:3] == Config.SID {
						local = append(local, uid)
					} else {
						remote = append(remote, uid)
					}
				}
			}
			if len(remote) > 0 {
				for sid := range IterFor(remote, msg.SenderID) {
					Debug.Printf("Forwarding PRIVMSG from %s to %s", msg.SenderID, sid)
					ircd.ToServer <- &Message{
						Prefix:  sender,
						Command: hook,
						Args: []string{
							channel.Name(),
							text,
						},
						DestIDs: []string{sid},
					}
				}
			}
			if len(local) > 0 {
				ircd.ToClient <- &Message{
					Prefix:  sender,
					Command: hook,
					Args: []string{
						channel.Name(),
						text,
					},
					DestIDs: local,
				}
			}
			continue
		}

		id, err := GetID(name)
		if num, ok := err.(*Numeric); ok {
			if !quiet {
				ircd.ToClient <- num.Message(msg.SenderID)
			}
			continue
		}
		if id[:3] == Config.SID {
			local = append(local, id)
		} else {
			remote = append(remote, id)
		}
	}
	if len(remote) > 0 {
		for _, remoteid := range remote {
			for sid := range IterFor([]string{remoteid}, "") {
				ircd.ToServer <- &Message{
					Prefix:  sender,
					Command: hook,
					Args: []string{
						remoteid,
						text,
					},
					DestIDs: []string{sid},
				}
			}
		}
	}
	if len(local) > 0 {
		ircd.ToClient <- &Message{
			Prefix:  sender,
			Command: hook,
			Args: []string{
				"*",
				text,
			},
			DestIDs: local,
		}
	}
}
