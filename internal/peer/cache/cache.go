package cache

import (
	"container/list"
	"sync"
)

type Segment struct {
	ID   string
	Data []byte
}

type entry struct {
	key string
	val Segment
}

type LRU struct {
	mu       sync.Mutex
	capacity int
	ll       *list.List
	items    map[string]*list.Element
}

func NewLRU(capacity int) *LRU {
	if capacity <= 0 {
		capacity = 16
	}
	return &LRU{
		capacity: capacity,
		ll:       list.New(),
		items:    make(map[string]*list.Element),
	}
}

func (l *LRU) Put(seg Segment) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if elem, ok := l.items[seg.ID]; ok {
		elem.Value.(*entry).val = seg
		l.ll.MoveToFront(elem)
		return
	}
	elem := l.ll.PushFront(&entry{key: seg.ID, val: seg})
	l.items[seg.ID] = elem
	if l.ll.Len() > l.capacity {
		l.removeOldest()
	}
}

func (l *LRU) Get(id string) (Segment, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if elem, ok := l.items[id]; ok {
		l.ll.MoveToFront(elem)
		return elem.Value.(*entry).val, true
	}
	return Segment{}, false
}

func (l *LRU) removeOldest() {
	elem := l.ll.Back()
	if elem == nil {
		return
	}
	l.ll.Remove(elem)
	ent := elem.Value.(*entry)
	delete(l.items, ent.key)
}

func (l *LRU) Keys() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	keys := make([]string, 0, len(l.items))
	for elem := l.ll.Front(); elem != nil; elem = elem.Next() {
		keys = append(keys, elem.Value.(*entry).key)
	}
	return keys
}
