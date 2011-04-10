package datastore

import (
	"testing"
)

import u "kevlar/ircd/util"

func TestLinkStoreNewLink(t *testing.T) {
	ls := NewLinkStore()
	success := NewReturn()
	ls.control <- NewLink{"SSSAAAAAA", 42, success}
	u.EQ(t,0, "success", true, <-success)

	lock,ok := ls.locks["SSSAAAAAA"]
	u.EQ(t,1, "lock exists", true, ok)
	u.NE(t,2, "lock", nil, lock)

	link,ok := ls.links["SSSAAAAAA"]
	u.EQ(t,3, "link exists", true, ok)
	u.EQ(t,4, "link", 42, link)
	close(ls.control)
}

func TestLinkStoreEditLink(t *testing.T) {
	ls := NewLinkStore()
	success := NewReturn()
	ls.control <- NewLink{"SSSAAAAAA", 42, success}
	<-success

	chk := make(map[int]bool)
	ls.control <- EditLink{"SSSAAAAAA", func(id string, l Link) bool {
		u.EQ(t,0, "editlink", 42, l)
		u.EQ(t,1, "editid", "SSSAAAAAA", id)
		chk[l.(int)] = true
		return true
	}, success}
	<-success
	u.EQ(t,2, "all", true, chk[42])

	close(ls.control)
}

func TestLinkStoreEachLink(t *testing.T) {
	ls := NewLinkStore()
	success := NewReturn()
	ls.control <- NewLink{"SSSAAAAAA", 42, success}
	<-success
	ls.control <- NewLink{"SSSAAAAAB", 43, success}
	<-success

	chklink := make(map[int]bool)
	chkid := make(map[string]bool)
	ls.control <- EachLink{func(id string, l Link) bool {
		chklink[l.(int)] = true
		chkid[id] = true
		return true
	}, success}
	<-success
	u.EQ(t,0, "link a", true, chklink[42])
	u.EQ(t,1, "link b", true, chklink[43])
	u.EQ(t,2, "id a", true, chkid["SSSAAAAAA"])
	u.EQ(t,3, "id b", true, chkid["SSSAAAAAB"])

	close(ls.control)
}

func BenchmarkLinkStoreControlLoop(b *testing.B) {
	ls := NewLinkStore()
	success := make(chan bool)
	for i := 0; i < b.N; i++ {
		ls.control <- Noop{success}
		<-success
	}
	close(ls.control)
}