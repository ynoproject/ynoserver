package main

import "sync"

var (
	sClientsMtx sync.RWMutex
	hClientsMtx sync.RWMutex
)

func getSessionClients() []*SessionClient {
	defer sClientsMtx.RUnlock()

	sClientsMtx.RLock()

	var clients []*SessionClient
	for _, client := range sessionClients {
		clients = append(clients, client)
	}

	return clients
}

func getSessionClientsLen() int {
	defer sClientsMtx.RUnlock()

	sClientsMtx.RLock()

	return len(sessionClients)
}

func getSessionClient(uuid string) (*SessionClient, bool) {
	defer sClientsMtx.RUnlock()

	sClientsMtx.RLock()

	client, ok := sessionClients[uuid]; return client,ok
}

func writeSessionClient(uuid string, client *SessionClient) {
	defer sClientsMtx.Unlock()

	sClientsMtx.Lock()

	sessionClients[uuid] = client
}

func deleteSessionClient(uuid string) {
	defer sClientsMtx.Unlock()

	sClientsMtx.Lock()

	delete(sessionClients, uuid)
}

func getHubClient(uuid string) (*Client, bool) {
	defer hClientsMtx.RUnlock()

	hClientsMtx.RLock()

	client, ok := hubClients[uuid]; return client, ok
}

func writeHubClient(uuid string, client *Client) {
	defer hClientsMtx.Unlock()

	hClientsMtx.Lock()

	hubClients[uuid] = client
}

func deleteHubClient(uuid string) {
	defer hClientsMtx.Unlock()

	hClientsMtx.Lock()

	delete(hubClients, uuid)
}

func (h *Hub) getId() int {
	defer h.idMtx.Unlock()

	// Find free id
	h.idMtx.RLock()
	
	var id int
	for {
		if !h.id[id] {
			break
		}

		id++
	}

	h.idMtx.RUnlock()

	// Mark id as used
	h.idMtx.Lock()

	h.id[id] = true

	return id
}

func (h *Hub) deleteId(id int) {
	defer h.idMtx.Unlock()

	h.idMtx.Lock()

	delete(h.id, id)
}
