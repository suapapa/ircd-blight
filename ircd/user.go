package ircd

import (
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	// Always lock this before locking a user mutex
	userMutex = new(sync.RWMutex)
	userMap   = make(map[string]*User)
	userIDs   = make(chan string)

	// userNicks[nick] = uid
	userNicks = make(map[string]string)
)

var (
	// The server prefix for all user IDs.  Set this before calling
	// NextUserID.
	UserIDPrefix = "000"
)

type userType int

const (
	UnregisteredUser userType = iota
	RegisteredAsUser
)

func genUserIDs() {
	chars := []byte{'A', 'A', 'A', 'A', 'A', 'A'}
	for {
		userIDs <- string(chars)
		for i := len(chars) - 1; i >= 0; i-- {
			switch chars[i] {
			case '9':
				chars[i] = 'A'
				continue
			case 'Z':
				chars[i] = '0'
			default:
				chars[i]++
			}
			break
		}
	}
}

// Store the user information and keep it synchronized across possible
// multiple accesses.
type User struct {
	mutex *sync.RWMutex
	ts    time.Time
	id    string
	user  string
	pass  string
	nick  string
	name  string
	utyp  userType
}

// Get the user ID.
func (u *User) ID() string {
	u.mutex.RLock()
	defer u.mutex.RUnlock()
	return u.id
}

// Get the user nick.
func (u *User) Nick() string {
	u.mutex.RLock()
	defer u.mutex.RUnlock()
	return u.nick
}

// Get the username (immutable).
func (u *User) User() string {
	return u.user
}

// Get the user's long name (immutable).
func (u *User) Name() string {
	return u.name
}

// Get the user's registration type (immutable).
func (u *User) Type() userType {
	return u.utyp
}

// Get the channel TS (comes as a string)
func (u *User) TS() string {
	u.mutex.RLock()
	defer u.mutex.RUnlock()
	return strconv.FormatInt(int64(u.ts.Second()), 10)
}

// Atomically get all of the user's information.
func (u *User) Info() (nick, user, name string, regType userType) {
	u.mutex.RLock()
	defer u.mutex.RUnlock()
	return u.nick, u.user, u.name, u.utyp
}

// Set the user's nick.
func (u *User) SetNick(nick string) error {
	if !ValidNick(nick) {
		return NewNumeric(ERR_ERRONEUSNICKNAME, nick)
	}

	lownick := ToLower(nick)

	userMutex.Lock()
	defer userMutex.Unlock()

	if id, used := userNicks[lownick]; used {
		if id == u.ID() {
			return nil
		}
		return NewNumeric(ERR_NICKNAMEINUSE, nick)
	}
	userNicks[lownick] = u.ID()

	lownick = ToLower(u.nick)
	delete(userNicks, lownick)

	u.mutex.Lock()
	defer u.mutex.Unlock()

	u.nick = nick
	u.ts = time.Now()
	return nil
}

// Set the user and gecos (immutable once set).
func (u *User) SetUser(user, name string) error {
	if len(u.user) > 0 {
		return NewNumeric(ERR_ALREADYREGISTRED)
	}

	if !ValidNick(user) || len(name) == 0 {
		// BUG(kevlar): Document this behavior
		return NewNumeric(ERR_NEEDMOREPARAMS)
	}

	u.mutex.Lock()
	defer u.mutex.Unlock()

	u.user, u.name = user, name
	u.ts = time.Now()
	return nil
}

// Set the user's type (immutable once set).
func (u *User) SetType(newType userType) error {
	if u.utyp != UnregisteredUser {
		return NewNumeric(ERR_ALREADYREGISTRED)
	}
	u.utyp = newType
	u.ts = time.Now()
	return nil
}

// Get the next available unique ID.
func NextUserID() string {
	return UserIDPrefix + <-userIDs
}

// Atomically retrieve user information.
func GetUserInfo(id string) (nick, user, name string, regType userType, ok bool) {
	userMutex.RLock()
	defer userMutex.RUnlock()

	var u *User
	if u, ok = userMap[id]; !ok {
		return
	}

	nick, user, name, regType = u.Info()
	return
}

// Get the ID for a particular nick.
func GetID(nick string) (id string, err error) {
	userMutex.RLock()
	defer userMutex.RUnlock()

	lownick := ToLower(nick)

	if _, ok := userMap[nick]; ok {
		return nick, nil
	}

	var ok bool
	if id, ok = userNicks[lownick]; !ok {
		err = NewNumeric(ERR_NOSUCHNICK, nick)
	}
	return
}

// GetUser the User structure for the given ID.  If it does not exist, it is
// created.
func GetUser(id string) *User {
	userMutex.Lock()
	defer userMutex.Unlock()

	// Database lookup?
	if u, ok := userMap[id]; ok {
		return u
	}

	u := &User{
		mutex: new(sync.RWMutex),
		id:    id,
		nick:  "*",
	}

	userMap[id] = u
	return u
}

// Delete the user record.
func Delete(id string) {
	userMutex.Lock()
	defer userMutex.Unlock()

	// Database lookup?
	if u, ok := userMap[id]; ok {
		u.mutex.RLock()
		defer u.mutex.RUnlock()

		nick := strings.ToLower(u.nick)
		delete(userNicks, nick)
		delete(userMap, id)
	}
}

func UserIter() <-chan string {
	userMutex.RLock()
	defer userMutex.RUnlock()

	out := make(chan string)
	ids := make([]string, 0, len(userMap))
	for _, u := range userMap {
		ids = append(ids, u.id)
	}

	go func() {
		defer close(out)
		for _, uid := range ids {
			out <- uid
		}
	}()
	return out
}

// UserSplit returns the list of users who will be unreachable when the given list
// of servers are split from the network.  It does not actually delete them.
func UserSplit(sids []string) (splitIDs []string) {
	gone := make(map[string]bool)
	for _, sid := range sids {
		gone[sid] = true
	}

	userMutex.RLock()
	defer userMutex.RUnlock()

	for _, u := range userMap {
		if gone[u.id[:3]] {
			splitIDs = append(splitIDs, u.id)
		}
	}
	return
}

func Import(uid, nick, user, host, ip, hops, ts, name string) error {
	userMutex.Lock()
	defer userMutex.Unlock()

	if _, ok := userMap[uid]; ok {
		return errors.New("UID collision")
	}

	lownick := ToLower(nick)

	if _, ok := userNicks[lownick]; ok {
		return errors.New("NICK collision")
	}

	its, _ := strconv.ParseInt(ts, 10, 64)
	u := &User{
		mutex: new(sync.RWMutex),
		ts:    time.Unix(0, its),
		id:    uid,
		user:  user,
		nick:  nick,
		name:  name,
		utyp:  RegisteredAsUser,
	}

	userMap[uid] = u
	userNicks[lownick] = uid
	return nil
}

func init() {
	go genUserIDs()
}
