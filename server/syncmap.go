/*
	Copyright (C) 2021-2024  The YNOproject Developers

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

type SyncMap[K comparable, V any] struct {
	Data map[K]V
	Mtx  sync.RWMutex
}

func NewSyncMap[K comparable, V any]() SyncMap[K, V] {
	return SyncMap[K, V]{Data: make(map[K]V)}
}

func (m *SyncMap[K, V]) Store(key K, value V) {
	m.Mtx.Lock()
	defer m.Mtx.Unlock()

	m.Data[key] = value
}

func (m *SyncMap[K, V]) Load(key K) V {
	m.Mtx.RLock()
	defer m.Mtx.RUnlock()

	return m.Data[key]
}

func (m *SyncMap[K, V]) LoadOK(key K) (V, bool) {
	m.Mtx.RLock()
	defer m.Mtx.RUnlock()

	value, ok := m.Data[key]
	return value, ok
}

func (m *SyncMap[K, V]) Exists(key K) bool {
	m.Mtx.RLock()
	defer m.Mtx.RUnlock()

	_, ok := m.Data[key]
	return ok
}

func (m *SyncMap[K, V]) Delete(key K) {
	m.Mtx.Lock()
	defer m.Mtx.Unlock()

	delete(m.Data, key)
}

func (m *SyncMap[K, V]) Len() int {
	m.Mtx.RLock()
	defer m.Mtx.RUnlock()

	return len(m.Data)
}
