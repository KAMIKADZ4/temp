package activity_tracker

import (
	"log"
	"encoding/json"
	"sync"
	"github.com/gofiber/contrib/websocket"
)

type empty struct{}

type SyncConn struct {
	C *websocket.Conn
	sync.Mutex
} 

type ActivityTracker struct {
	userConn map[int]*SyncConn
	docUsers map[string]map[int]empty
	userDoc map[int]string
	wg sync.WaitGroup
	mu sync.RWMutex
}

func NewActivityTracker() *ActivityTracker {
	return &ActivityTracker{
		userConn: make(map[int]*SyncConn),
		docUsers: make(map[string]map[int]empty),
		userDoc: make(map[int]string),
	}
}

func (at *ActivityTracker) Wait() {
	at.wg.Wait()
}

func (at *ActivityTracker) notifyDoc(doc string) {
	defer at.wg.Done()

	at.mu.RLock()
	defer at.mu.RUnlock()

	usersInDoc := make([]int, 0, len(at.docUsers[doc]))
	for userId := range at.docUsers[doc] {
		usersInDoc = append(usersInDoc, userId)
	}
	
	msg, err := json.Marshal(usersInDoc)
	if err != nil {
		log.Println(
			"Failed to marshall data while notify ", doc, 
			" Error: ", err,
		)
	}

	var wg sync.WaitGroup
	for userId := range at.docUsers[doc] {
		if conn, exists := at.userConn[userId]; exists {
			wg.Add(1)
			go func(conn *SyncConn) {
				defer wg.Done()
				conn.Lock()
				defer conn.Unlock()
				if err := conn.C.WriteMessage(websocket.TextMessage, msg); err != nil {
					log.Println("Write error:", err)
					conn.C.Close()
				}
			}(conn)
		}
	}
	wg.Wait()
}

func (at *ActivityTracker) AddUser(userId int, conn *SyncConn, doc string) {
	at.mu.Lock()
	defer at.mu.Unlock()

	if prevDoc, exists := at.userDoc[userId]; exists && prevDoc != doc {
		delete(at.docUsers[prevDoc], userId)
		at.wg.Add(1)
		go at.notifyDoc(prevDoc)
		// at.notifyDoc(prevDoc)
	}

	at.userConn[userId] = conn
	at.userDoc[userId] = doc

	if _, exists := at.docUsers[doc]; !exists {
		at.docUsers[doc] = make(map[int]empty)
	}
	at.docUsers[doc][userId] = empty{}

	at.wg.Add(1)
	go at.notifyDoc(doc)
	// at.notifyDoc(doc)
}

func (at *ActivityTracker) RemoveUser(userId int) {
	at.mu.Lock()
    defer at.mu.Unlock()

	if doc, exists := at.userDoc[userId]; exists {
		delete(at.userDoc, userId)
		delete(at.userConn, userId)

		delete(at.docUsers[doc], userId)
		if len(at.docUsers[doc]) == 0 {
			delete(at.docUsers, doc)
		} else {
			at.wg.Add(1)
			go at.notifyDoc(doc)
			// at.notifyDoc(doc)
		}		
	}	
}
