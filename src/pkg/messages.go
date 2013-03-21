package ircd

import (
	"bytes"
	"log"
)

// Construct a names message for the channel.
func (c *Channel) NamesMessage(destIDs ...string) *Message {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	buf := bytes.NewBuffer(nil)
	for id := range c.users {
		nick, _, _, _, ok := GetUserInfo(id)
		if !ok {
			log.Printf("Warning: Unknown id %q in %s", id, c.name)
			continue
		}
		buf.WriteByte(' ')
		buf.WriteString(nick)
	}
	buf.ReadByte()
	return &Message{
		Command: RPL_NAMREPLY,
		Args: []string{
			// =public *private @secret
			"*",
			"@",
			c.name,
			buf.String(),
		},
		DestIDs: destIDs,
	}
}
