/*
	Copyright (C) 2021-2023  The YNOproject Developers

	This program is free software: you can redistribute it and/or modify
	it under the terms of the GNU Affero General Public License as published by
	the Free Software Foundation, either version 3 of the License, or
	(at your option) any later version.

	This program is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU Affero General Public License for more details.

	You should have received a copy of the GNU Affero General Public License
	along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package server

import "sync"

type ClientMap struct {
	clients map[string]*SessionClient
	mutex   sync.RWMutex
}

func NewSCMap() *ClientMap {
	return &ClientMap{
		clients: make(map[string]*SessionClient),
	}
}

func (m *ClientMap) Store(uuid string, client *SessionClient) {
	m.mutex.Lock()

	m.clients[uuid] = client

	m.mutex.Unlock()
}

func (m *ClientMap) Load(uuid string) (*SessionClient, bool) {
	m.mutex.RLock()

	client, ok := m.clients[uuid]

	m.mutex.RUnlock()

	return client, ok
}

func (m *ClientMap) Delete(uuid string) {
	m.mutex.Lock()

	delete(m.clients, uuid)

	m.mutex.Unlock()
}

func (m *ClientMap) Get() []*SessionClient {
	m.mutex.RLock()

	var clients []*SessionClient
	for _, client := range m.clients {
		clients = append(clients, client)
	}

	m.mutex.RUnlock()

	return clients
}

func (m *ClientMap) GetAmount() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return len(m.clients)
}

func (m *ClientMap) Exists(uuid string) bool {
	m.mutex.RLock()

	_, ok := m.clients[uuid]

	m.mutex.RUnlock()

	return ok
}
