package proxy

import "sync"

const copyBufferSize = 32 * 1024

// copyBufferPool lends fixed size copy buffers to httputil.ReverseProxy so
// each proxied response does not allocate its own.
type copyBufferPool struct {
	pool sync.Pool
}

func newCopyBufferPool() *copyBufferPool {
	return &copyBufferPool{pool: sync.Pool{New: func() any { return new([copyBufferSize]byte) }}}
}

// Get returns a buffer of copyBufferSize bytes.
func (p *copyBufferPool) Get() []byte {
	return p.pool.Get().(*[copyBufferSize]byte)[:]
}

// Put returns a buffer obtained from Get. Buffers of any other size are dropped.
func (p *copyBufferPool) Put(b []byte) {
	if cap(b) != copyBufferSize {
		return
	}
	p.pool.Put((*[copyBufferSize]byte)(b[:copyBufferSize]))
}

var sharedCopyBufferPool = newCopyBufferPool()
