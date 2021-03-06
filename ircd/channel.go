package ircd

import (
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
	ts    time.Time
	users map[string]string // users[uid] = hostmask
}

// GetChannel the Channel structure for the given channel.  If it does not exist and
// create is true, it is created.
func GetChannel(name string, create bool) (*Channel, error) {
	chanMutex.Lock()
	defer chanMutex.Unlock()

	if !ValidChannel(name) {
		return nil, NewNumeric(ERR_NOSUCHCHANNEL, name)
	}

	lowname := ToLower(name)

	// Database lookup?
	if c, ok := chanMap[lowname]; ok {
		return c, nil
	} else if !create {
		return nil, NewNumeric(ERR_NOSUCHCHANNEL, name)
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
	return strconv.FormatInt(int64(c.ts.Second()), 10)
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
func (c *Channel) Join(uids ...string) (notify []string, err error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for _, uid := range uids {
		if _, on := c.users[uid]; on {
			return nil, NewNumeric(ERR_USERONCHANNEL, uid, c.name)
		}

		// TODO(kevlar): Check hostmask
		c.users[uid] = "host@mask"
		c.ts = time.Now()
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
//  - User 2 gets the *Channel from GetChannel()
//  - User 1 finishes parting and #chan is deleted
//  - User 2 joins the nonexistent channel
// Possible solutions:
//  - Make JOIN and PART global (most thorough)
//  - Check channel existence and recreate after unlock (easiest)
// DONE.  Verify?

// Part a user from the channel.
func (c *Channel) Part(uid string) (notify []string, err error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if _, on := c.users[uid]; !on {
		return nil, NewNumeric(ERR_NOTONCHANNEL, c.name)
	}

	notify = make([]string, 0, len(c.users))
	for id := range c.users {
		notify = append(notify, id)
	}
	delete(c.users, uid)
	c.ts = time.Now()

	if len(c.users) == 0 {
		chanMutex.Lock()
		defer chanMutex.Unlock()

		delete(chanMap, c.name)
	}

	return
}

func PartAll(uid string) (notify map[string][]string) {
	chanMutex.Lock()
	defer chanMutex.Unlock()

	notify = make(map[string][]string)

	part := func(c *Channel) {
		c.mutex.Lock()
		defer c.mutex.Unlock()

		if _, on := c.users[uid]; !on {
			return
		}

		for id := range c.users {
			notify[c.name] = append(notify[c.name], id)
		}
		delete(c.users, uid)
		c.ts = time.Now()

		if len(c.users) == 0 {
			delete(chanMap, c.name)
		}
	}

	for _, c := range chanMap {
		part(c)
	}

	return
}

// ChanSplit removes the given uids from all channels and returns the users from
// the given server who should be notified of the splits in a map of splitting
// user to a list of that user's peers.
func ChanSplit(sid string, uids []string) map[string][]string {
	leaving2notify := make(map[string]map[string]bool)
	for _, uid := range uids {
		leaving2notify[uid] = make(map[string]bool)
	}

	chanMutex.Lock()
	defer chanMutex.Unlock()

	split := func(c *Channel) {
		c.mutex.Lock()
		defer c.mutex.Unlock()

		leavingChanUIDs := []string{}
		for leavingUID := range leaving2notify {
			leavingChanUIDs = append(leavingChanUIDs, leavingUID)
			delete(c.users, leavingUID)
		}
		if len(leavingChanUIDs) == 0 {
			return
		}

		for peerUID := range c.users {
			if sid != peerUID[:3] {
				continue
			}
			for _, leavingUID := range leavingChanUIDs {
				leaving2notify[leavingUID][peerUID] = true
			}
		}

		if len(c.users) == 0 {
			delete(chanMap, c.name)
		}
	}

	for _, c := range chanMap {
		split(c)
	}

	notify := make(map[string][]string)
	for gone, peers := range leaving2notify {
		for peer := range peers {
			notify[gone] = append(notify[gone], peer)
		}
	}
	return notify
}

func ChannelIter() <-chan string {
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
