package ircd

import (
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"
)

// For the purposes of this file, think of the current server as the root of a
// tree where each node has its links hanging below it.  Downstream refers to
// any server directly connected below the node, upstream refers to the single
// server above the node.

// servMap[SID] = &Server{...}
// - Stores the information for any server
// upstream[remote SID] = upstream SID
// - Maps a logical server to the peer to which it's linked
// - Locally linked servers aren't in this map.
// downstream[sid1][sid2] = bool
// - if downstream[sid1][sid2] == true, sid2 is directly downstream of sid1
var (
	servMap    = make(map[string]*Server)
	upstream   = make(map[string]string)
	downstream = make(map[string]map[string]bool)
	servMutex  = new(sync.RWMutex)
)

type servType int

const (
	UnregisteredServer servType = iota
	RegisteredAsServer
)

type Server struct {
	mutex  *sync.RWMutex
	id     string
	ts     time.Time
	styp   servType
	pass   string
	desc   string
	sver   int
	server string
	link   string
	capab  []string
	hops   int
}

func (s *Server) ID() string {
	return s.id
}

func (s *Server) Type() servType {
	return s.styp
}

// Atomically get all of the server's information.
func (s *Server) Info() (sid, server, pass string, capab []string) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.id, s.server, s.pass, s.capab
}

// Get retrieves and/or creates a server.  If a server is created,
// it is directly linked to this one.
func GetServer(id string, create bool) *Server {
	servMutex.Lock()
	defer servMutex.Unlock()

	if s, ok := servMap[id]; ok {
		return s
	}

	if !create {
		return nil
	}

	s := &Server{
		mutex: new(sync.RWMutex),
		id:    id,
	}
	servMap[id] = s
	downstream[id] = make(map[string]bool)
	return s
}

// LinkServer registers a new server linked behind link.
func LinkServer(link, sid, name, hops, desc string) error {
	servMutex.Lock()
	defer servMutex.Unlock()

	if _, ok := servMap[sid]; ok {
		return errors.New("Server already linked: " + sid)
	}

	ihops, _ := strconv.Atoi(hops)

	s := &Server{
		mutex:  new(sync.RWMutex),
		id:     sid,
		server: name,
		desc:   desc,
		link:   link,
		hops:   ihops,
	}

	servMap[sid] = s
	upstream[sid] = link
	downstream[link][sid] = true
	downstream[sid] = make(map[string]bool)

	up := link
	chain := 1
	for len(up) > 0 {
		up, _ = upstream[up]
		chain++
	}

	Info.Printf("Server %s (%s) linked behind %s", name, sid, link)
	if chain != ihops {
		Warn.Printf("%s: %d hops found, %d hops reported", sid, chain, ihops)
	}

	return nil
}

// Atomically get all of the server's information.
func GetServerInfo(id string) (sid, server string, capab []string, typ servType, ok bool) {
	servMutex.Lock()
	defer servMutex.Unlock()

	var s *Server
	if s, ok = servMap[id]; !ok {
		return
	}

	return s.id, s.server, s.capab, s.styp, true
}

// ServerIter iterates over all server links
func ServerIter() <-chan string {
	servMutex.RLock()
	defer servMutex.RUnlock()

	out := make(chan string)
	links := make([]string, 0, len(downstream))
	for link := range servMap {
		if _, skip := upstream[link]; skip {
			continue
		}
		links = append(links, link)
	}

	go func() {
		defer close(out)
		for _, sid := range links {
			out <- sid
		}
	}()
	return out
}

// IterFor iterates over the link IDs for all of the ID in the given list.
// The list may contain SIDs, UIDs, or both.  If the skipLink is given,
// any servers behind that link will be skipped.
func IterFor(ids []string, skipLink string) <-chan string {
	servMutex.RLock()
	defer servMutex.RUnlock()

	for {
		if actual, ok := upstream[skipLink]; ok {
			skipLink = actual
		} else {
			break
		}
	}

	out := make(chan string)
	links := []string{}
	cache := make(map[string]bool)

nextId:
	for _, id := range ids {
		sid := id[:3]
		for {
			if cache[sid] {
				continue nextId
			}
			cache[sid] = true
			if actual, ok := upstream[sid]; ok {
				sid = actual
			} else {
				break
			}
		}
		if sid == skipLink {
			continue nextId
		}
		links = append(links, sid)
	}

	go func() {
		defer close(out)
		for _, sid := range links {
			out <- sid
		}
	}()
	return out
}

func (s *Server) SetType(typ servType) error {
	if s.styp != UnregisteredServer {
		return errors.New("Already registered")
	}

	s.styp = typ
	return nil
}

func (s *Server) SetPass(password, ts, prefix string) error {
	if len(password) == 0 {
		return errors.New("Zero-length password")
	}

	if ts != "6" {
		return errors.New("TS " + ts + " is unsupported")
	}

	if !ValidServerPrefix(prefix) {
		return errors.New("SID " + prefix + " is invalid")
	}

	s.pass, s.sver = password, 6
	s.ts = time.Now()
	return nil
}

func (s *Server) SetCapab(capab string) error {
	if !strings.Contains(capab, "QS") {
		return errors.New("QS CAPAB missing")
	}

	if !strings.Contains(capab, "ENCAP") {
		return errors.New("ENCAP CAPAB missing")
	}

	s.capab = strings.Fields(capab)
	s.ts = time.Now()
	return nil
}

func (s *Server) SetServer(serv, hops string) error {
	if len(serv) == 0 {
		return errors.New("Zero-length server name")
	}

	if hops != "1" {
		return errors.New("Hops = " + hops + " is unsupported")
	}

	s.server, s.hops = serv, 1
	s.ts = time.Now()
	return nil
}

// IsLocal returns true if the SID is locally linked
func IsLocal(sid string) bool {
	if _, remote := upstream[sid]; remote {
		return false
	}
	if _, exists := downstream[sid]; !exists {
		return false
	}
	return true
}

// Return the SIDs of all servers behind the given link, starting with the
// server itself.  If the server is unknown, the returned list is empty.
func LinkedTo(link string) []string {
	servMutex.Lock()
	defer servMutex.Unlock()

	return linkedTo(link)
}

// Make sure the server mutex is (r)locked before calling this.
func linkedTo(link string) []string {
	if _, ok := servMap[link]; !ok {
		Warn.Printf("Mapping nonexistent link %s", link)
		return nil
	}

	sids := []string{link}
	for downstream := range downstream[link] {
		sids = append(sids, linkedTo(downstream)...)
	}

	return sids
}

// Unlink deletes the given server and all servers behind it.  It returns the list
// of SIDs that were split.
func Unlink(split string) (sids []string) {
	servMutex.Lock()
	defer servMutex.Unlock()

	sids = linkedTo(split)

	for _, sid := range sids {
		Info.Printf("Split %s: Unlinking %s", split, sid)

		// Delete the server entry
		delete(servMap, sid)

		// Unlink from the upstream server's downstream list
		if up, ok := upstream[sid]; ok {
			// But only if the upstream server is still around
			if _, ok := downstream[up]; ok {
				delete(downstream[up], sid)
			}
		}

		// Remove the server's downstream list
		delete(downstream, sid)
	}

	return
}
