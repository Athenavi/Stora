package middleware

import (
	"io"
	"sync"
	"time"
)

// TokenBucket 实现令牌桶流量整形，用于上传/下载限速。
// 每个用户一个桶，以 bytes/s 为单位。
type TokenBucket struct {
	mu         sync.Mutex
	rate       int64   // bytes per second
	capacity   int64   // burst capacity
	tokens     int64
	lastRefill time.Time
}

// NewTokenBucket 创建令牌桶。
// rate: 每秒允许的字节数，0 = 不限速。
func NewTokenBucket(rate int64) *TokenBucket {
	if rate <= 0 {
		return &TokenBucket{rate: 0, capacity: 0, tokens: 0}
	}
	return &TokenBucket{
		rate:       rate,
		capacity:   rate,     // 初始容量 = 1秒的额度
		tokens:     rate,     // 初始满令牌
		lastRefill: time.Now(),
	}
}

// Wait 阻塞直到获取 n 个字节的令牌。
func (tb *TokenBucket) Wait(n int) {
	if tb.rate <= 0 {
		return // 不限速
	}

	tb.mu.Lock()
	tb.refill()
	for tb.tokens < int64(n) {
		needed := int64(n) - tb.tokens
		waitDuration := time.Duration(needed*1000/tb.rate) * time.Millisecond
		if waitDuration < time.Millisecond {
			waitDuration = time.Millisecond
		}
		tb.mu.Unlock()
		time.Sleep(waitDuration)
		tb.mu.Lock()
		tb.refill()
	}
	tb.tokens -= int64(n)
	tb.mu.Unlock()
}

func (tb *TokenBucket) refill() {
	if tb.rate <= 0 {
		return
	}
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill)
	tb.lastRefill = now
	addTokens := int64(elapsed.Seconds() * float64(tb.rate))
	if addTokens > 0 {
		tb.tokens += addTokens
		if tb.tokens > tb.capacity {
			tb.tokens = tb.capacity
		}
	}
}

// RateLimitedReader wraps an io.Reader with token bucket rate limiting.
type RateLimitedReader struct {
	reader io.Reader
	bucket *TokenBucket
}

func (r *RateLimitedReader) Read(p []byte) (int, error) {
	if r.bucket.rate <= 0 {
		return r.reader.Read(p)
	}
	r.bucket.Wait(len(p))
	return r.reader.Read(p)
}

// RateLimitedWriter wraps an io.Writer with token bucket rate limiting.
type RateLimitedWriter struct {
	writer io.Writer
	bucket *TokenBucket
}

func (w *RateLimitedWriter) Write(p []byte) (int, error) {
	if w.bucket.rate <= 0 {
		return w.writer.Write(p)
	}
	w.bucket.Wait(len(p))
	return w.writer.Write(p)
}

// ── 用户级别的 Bucket 管理器 ──────────────────────

// SpeedLimiter 管理所有用户的令牌桶。
type SpeedLimiter struct {
	mu          sync.RWMutex
	buckets     map[int64]*TokenBucket // userID → bucket
	uploadRate  int64                  // bytes/s
	downloadRate int64                 // bytes/s
}

// NewSpeedLimiter 创建限速器。
// uploadKBps, downloadKBps: 每秒千字节数，0=不限速。
func NewSpeedLimiter(uploadKBps, downloadKBps int64) *SpeedLimiter {
	return &SpeedLimiter{
		buckets:      make(map[int64]*TokenBucket),
		uploadRate:   uploadKBps * 1024,
		downloadRate: downloadKBps * 1024,
	}
}

func (sl *SpeedLimiter) GetUploadBucket(userID int64) *TokenBucket {
	sl.mu.RLock()
	b, ok := sl.buckets[userID]
	sl.mu.RUnlock()
	if ok {
		return b
	}
	sl.mu.Lock()
	defer sl.mu.Unlock()
	if b, ok := sl.buckets[userID]; ok {
		return b
	}
	b = NewTokenBucket(sl.uploadRate)
	sl.buckets[userID] = b
	return b
}

func (sl *SpeedLimiter) GetDownloadBucket(userID int64) *TokenBucket {
	sl.mu.RLock()
	// Use negative userID as key for download buckets to avoid collision
	b, ok := sl.buckets[-userID]
	sl.mu.RUnlock()
	if ok {
		return b
	}
	sl.mu.Lock()
	defer sl.mu.Unlock()
	if b, ok := sl.buckets[-userID]; ok {
		return b
	}
	b = NewTokenBucket(sl.downloadRate)
	sl.buckets[-userID] = b
	return b
}

// WrapDownloadReader wraps a reader with download speed limiting.
func (sl *SpeedLimiter) WrapDownloadReader(userID int64, reader io.Reader) io.Reader {
	if sl.downloadRate <= 0 {
		return reader
	}
	return &RateLimitedReader{reader: reader, bucket: sl.GetDownloadBucket(userID)}
}

// WrapUploadWriter wraps a writer with upload speed limiting.
// Note: For uploads we typically need to limit the reader side.
func (sl *SpeedLimiter) WrapUploadReader(userID int64, reader io.Reader) io.Reader {
	if sl.uploadRate <= 0 {
		return reader
	}
	return &RateLimitedReader{reader: reader, bucket: sl.GetUploadBucket(userID)}
}
