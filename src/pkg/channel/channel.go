package channel

import (
	"kevlar/ircd/parser"
	"os"
	"strconv"
	"sync"
	"time"
)

var (
	// Always lock this after locking a channel mutex if both are to be locked.
	chanMutex = new(sync.RWMutex)
	chanMap   = make(map[string]*Channel)
)

// Store the channel information and keep it synchronized across possible
// multiple accesses.
type Channel struct {
	mutex *sync.RWMutex
	name  string
	ts    int64
	users map[string]string // users[uid] = hostmask
}

// Get the Channel structure for the given channel.  If it does not exist and
// create is true, it is created.
func Get(name string, create bool) (*Channel, os.Error) {
	chanMutex.Lock()
	defer chanMutex.Unlock()

	if !parser.ValidChannel(name) {
		return nil, parser.NewNumeric(parser.ERR_NOSUCHCHANNEL, name)
	}

	lowname := parser.ToLower(name)

	// Database lookup?
	if c, ok := chanMap[lowname]; ok {
		return c, nil
	} else if !create {
		return nil, parser.NewNumeric(parser.ERR_NOSUCHCHANNEL, name)
	}

	c := &Channel{
		mutex: new(sync.RWMutex),
		name:  name,
		users: make(map[string]string),
	}

	chanMap[lowname] = c
	return c, nil
}

// Get the channel name (immutable).
func (c *Channel) Name() string {
	return c.name
}

// Get the channel TS (comes as a string)
func (c *Channel) TS() string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return strconv.Itoa64(c.ts / 1e9)
}

// Get the chanel member IDs
func (c *Channel) UserIDs() []string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	ids := make([]string, 0, len(c.users))
	for id := range c.users {
		ids = append(ids, id)
	}
	return ids
}

// Get the chanel member IDs with prefixes
func (c *Channel) UserIDsWithPrefix() []string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	ids := make([]string, 0, len(c.users))
	for id := range c.users {
		ids = append(ids, id)
	}
	return ids
}

// Get whether a user is on the channel.
func (c *Channel) OnChan(uid string) (on bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	_, on = c.users[uid]
	return
}

// Join a user to the channel.
func (c *Channel) Join(uids ...string) (notify []string, err os.Error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for _, uid := range uids {
		if _, on := c.users[uid]; on {
			return nil, parser.NewNumeric(parser.ERR_USERONCHANNEL, uid, c.name)
		}

		// TODO(kevlar): Check hostmask
		c.users[uid] = "host@mask"
		c.ts = time.Nanoseconds()
	}

	notify = make([]string, 0, len(c.users))
	for id := range c.users {
		notify = append(notify, id)
	}

	// Make sure that this channel exists (bad news if it doesn't)
	chanMutex.Lock()
	defer chanMutex.Unlock()
	if _, exist := chanMap[c.name]; !exist {
		chanMap[c.name] = c
	}

	return
}

// TODO(kevlar): Eliminate race condition:
//  - User 1 starts parting #chan
//  - User 2 gets the *Channel from Get()
//  - User 1 finishes parting and #chan is deleted
//  - User 2 joins the nonexistent channel
// Possible solutions:
//  - Make JOIN and PART global (most thorough)
//  - Check channel existence and recreate after unlock (easiest)
// DONE.  Verify?

// Part a user from the channel.
func (c *Channel) Part(uid string) (notify []string, err os.Error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if _, on := c.users[uid]; !on {
		return nil, parser.NewNumeric(parser.ERR_NOTONCHANNEL, c.name)
	}

	notify = make([]string, 0, len(c.users))
	for id := range c.users {
		notify = append(notify, id)
	}
	c.users[uid] = "", false
	c.ts = time.Nanoseconds()

	if len(c.users) == 0 {
		chanMutex.Lock()
		defer chanMutex.Unlock()

		chanMap[c.name] = nil, false
	}

	return
}

func Iter() <-chan string {
	chanMutex.RLock()
	defer chanMutex.RUnlock()

	out := make(chan string)
	ids := make([]string, 0, len(chanMap))
	for _, c := range chanMap {
		ids = append(ids, c.name)
	}

	go func() {
		defer close(out)
		for _, channel := range ids {
			out <- channel
		}
	}()
	return out
}
