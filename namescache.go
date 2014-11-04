package main

import (
	"sync"
	"sync/atomic"
)

type namesCache struct {
	users           map[Userid]*User
	marshallednames []byte
	usercount       uint32
	rooms           map[string]map[Userid]*User
	marshalledrooms map[string][]byte
	ircnames        [][]string
	sync.RWMutex
}

type userChan struct {
	user *User
	c    chan *User
}

type NamesOut struct {
	Users       []*SimplifiedUser `json:"users"`
	Connections uint32            `json:"connectioncount"`
}

var namescache = namesCache{
	users:   make(map[Userid]*User),
	RWMutex: sync.RWMutex{},
	rooms:   make(map[string]map[Userid]*User),
	marshalledrooms: make(map[string][]byte),
}

func initNamesCache() {
}

func (nc *namesCache) getIrcNames() [][]string {
	nc.RLock()
	defer nc.RUnlock()
	return nc.ircnames
}

func (nc *namesCache) marshalRoom(room string) {
	users := make([]*SimplifiedUser, 0, len(nc.rooms[room]))
	for _, u := range nc.rooms[room] {
		u.RLock()
		if u.connections <= 0 {
			continue
		}
		users = append(users, u.simplified)
	}

	nc.marshalledrooms[room], _ = Marshal(&NamesOut{
		Users:       users,
		Connections: nc.usercount,
	})

	for _, u := range nc.rooms[room] {
		u.RUnlock()
	}

}

func (nc *namesCache) marshalNames(updateircnames bool) {
	users := make([]*SimplifiedUser, 0, len(nc.users))
	var allnames []string
	if updateircnames {
		allnames = make([]string, 0, len(nc.users))
	}
	for _, u := range nc.users {
		u.RLock()
		if u.connections <= 0 {
			continue
		}
		users = append(users, u.simplified)
		if updateircnames {
			prefix := ""
			switch {
			case u.featureGet(ISADMIN):
				prefix = "~" // +q
			case u.featureGet(ISBOT):
				prefix = "&" // +a
			case u.featureGet(ISMODERATOR):
				prefix = "@" // +o
			case u.featureGet(ISVIP):
				prefix = "%" // +h
			case u.featureGet(ISSUBSCRIBER):
				prefix = "+" // +v
			}
			allnames = append(allnames, prefix+u.nick)
		}
	}

	if updateircnames {
		l := 0
		var namelines [][]string
		var names []string
		for _, name := range allnames {
			if l+len(name) > 400 {
				namelines = append(namelines, names)
				l = 0
				names = nil
			}
			names = append(names, name)
			l += len(name)
		}
		nc.ircnames = namelines
	}

	nc.marshallednames, _ = Marshal(&NamesOut{
		Users:       users,
		Connections: nc.usercount,
	})

	for _, u := range nc.users {
		u.RUnlock()
	}
}

func (nc *namesCache) getNames() []byte {
	nc.RLock()
	defer nc.RUnlock()
	return nc.marshallednames
}

func (nc *namesCache) getNamesInRoom(room string) []byte {
	nc.RLock()
	defer nc.RUnlock()
	return nc.marshalledrooms[room]
}

func (nc *namesCache) add(user *User) *User {
	nc.Lock()
	defer nc.Unlock()

	nc.usercount++
	var updateircnames bool
	if u, ok := nc.users[user.id]; ok {
		atomic.AddInt32(&u.connections, 1)
	} else {
		updateircnames = true
		user.connections++
		su := &SimplifiedUser{
			Nick:     user.nick,
			Features: user.simplified.Features,
		}
		user.simplified = su
		nc.users[user.id] = user
	}
	nc.marshalNames(updateircnames)
	return nc.users[user.id]
}

func (nc *namesCache) disconnect(c *Connection) {
	user := c.user
	nc.Lock()
	defer nc.Unlock()
	nc.usercount--

	if user == nil {
		return
	}

	room := nc.rooms[c.room]
	if room == nil {
		return
	}

	u := room[user.id]
	if u == nil {
		return
	}

	atomic.AddInt32(&u.connections, -1)
	// we do not delete the users so that the lastmessage is preserved for
	// anti-spam purposes, sadly this means memory usage can only go up
	nc.marshalRoom(c.room)
}

func (nc *namesCache) refresh(user *User) {
	nc.RLock()
	defer nc.RUnlock()

	if u, ok := nc.users[user.id]; ok {
		u.Lock()
		u.simplified.Nick = user.nick
		u.simplified.Features = user.simplified.Features
		u.nick = user.nick
		u.features = user.features
		u.Unlock()
		nc.marshalNames(true)
	}
}

func (nc *namesCache) addConnection(c *Connection) {
	nc.Lock()
	defer nc.Unlock()
	nc.usercount++
	user := c.user

	if user == nil {
		return
	}

	room := nc.rooms[c.room]
	if room == nil {
		room = make(map[Userid]*User)
		nc.rooms[c.room] = room
	}

	u := room[user.id]
	if u == nil {
		su := &SimplifiedUser{
			Nick:     user.nick,
			Features: user.simplified.Features,
		}
		user.simplified = su
		u = user
		room[user.id] = u
	}

	atomic.AddInt32(&u.connections, 1)
	// assign back to avoid duplicate users
	c.user = u
	nc.marshalRoom(c.room)
}
