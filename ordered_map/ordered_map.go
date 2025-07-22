package orderedmap

type OrderedMap[K comparable, V any] struct {
	underlying map[K]V
	order      []K
}

func NewOrderedMap[K comparable, V any]() *OrderedMap[K, V] {
	return &OrderedMap[K, V]{
		underlying: make(map[K]V),
		order:      make([]K, 0),
	}
}

func (m *OrderedMap[K, V]) Set(key K, value V) {
	m.underlying[key] = value
	m.order = append(m.order, key)
}

func (m *OrderedMap[K, V]) Get(key K) (V, bool) {
	value, ok := m.underlying[key]
	return value, ok
}

func (m *OrderedMap[K, V]) Delete(key K) {
	delete(m.underlying, key)
	for i, k := range m.order {
		if k == key {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
}

func (m *OrderedMap[K, V]) GetPlace(key K) int {
	for i, k := range m.order {
		if k == key {
			return i
		}
	}
	
	return -1
}

func (m *OrderedMap[K, V]) Keys() []K {
	return m.order
}

func (m *OrderedMap[K, V]) Values() []V {
	values := make([]V, len(m.order))
	for i, k := range m.order {
		values[i] = m.underlying[k]
	}
	return values
}

func (m *OrderedMap[K, V]) Len() int {
	return len(m.order)
}