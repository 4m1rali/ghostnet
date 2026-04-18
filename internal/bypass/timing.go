package bypass

import (
	"math"
	mrand "math/rand"
	"sync"
	"time"
)

var timingRNG struct {
	sync.Mutex
	r *mrand.Rand
}

func init() {
	timingRNG.r = mrand.New(mrand.NewSource(time.Now().UnixNano()))
}

func tRandFloat64() float64 {
	timingRNG.Lock()
	v := timingRNG.r.Float64()
	timingRNG.Unlock()
	return v
}

func tRandNormFloat64() float64 {
	timingRNG.Lock()
	v := timingRNG.r.NormFloat64()
	timingRNG.Unlock()
	return v
}

func HumanDelay(baseMs, minMs, maxMs float64) time.Duration {
	if minMs <= 0 {
		minMs = 0.1
	}
	if maxMs <= 0 {
		maxMs = 50.0
	}

	var delayMs float64
	if tRandFloat64() < 0.08 {
		u := tRandFloat64()
		if u >= 1.0 {
			u = 0.9999
		}
		delayMs = 8.0*math.Pow(-math.Log(1.0-u), 1.0/1.5) + 5.0
	} else {
		sigma := baseMs * 0.30
		delayMs = baseMs + tRandNormFloat64()*sigma
	}

	if delayMs < minMs {
		delayMs = minMs
	}
	if delayMs > maxMs {
		delayMs = maxMs
	}
	return time.Duration(delayMs * float64(time.Millisecond))
}

func AdaptiveDelay(synTime time.Time, baseMs, minMs, maxMs float64) time.Duration {
	target := HumanDelay(baseMs, minMs, maxMs)
	remaining := target - time.Since(synTime)
	if remaining < 0 {
		return 0
	}
	return remaining
}

func JitterSleep(base time.Duration, jitterMs float64) {
	if jitterMs <= 0 {
		time.Sleep(base)
		return
	}
	jitter := time.Duration((tRandFloat64()*2-1)*jitterMs*float64(time.Millisecond))
	d := base + jitter
	if d < 0 {
		return
	}
	time.Sleep(d)
}

func ExponentialBackoff(attempt int, baseMs, maxMs float64) time.Duration {
	if baseMs <= 0 {
		baseMs = 100
	}
	if maxMs <= 0 {
		maxMs = 5000
	}
	delay := baseMs * math.Pow(2, float64(attempt))
	jitter := tRandFloat64() * baseMs * 0.5
	delay += jitter
	if delay > maxMs {
		delay = maxMs
	}
	return time.Duration(delay * float64(time.Millisecond))
}
