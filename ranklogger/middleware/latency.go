package middleware

import (
	"github.com/gofiber/fiber/v2"
	"sort"
	"sync"
	"time"
)

type LatencyStats struct {
	mu      sync.Mutex
	samples []float64
	maxSize int
}

func NewLatencyStats(size int) *LatencyStats {
	return &LatencyStats{
		maxSize: size,
		samples: make([]float64, 0, size),
	}
}

func (s *LatencyStats) Record(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// ミリ秒単位で記録
	s.samples = append(s.samples, float64(d.Microseconds())/1000.0)

	// 古いデータを捨てる（リングバッファ的な簡易実装）
	if len(s.samples) > s.maxSize {
		s.samples = s.samples[1:]
	}
}

func (s *LatencyStats) GetPercentile(p float64) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.samples) == 0 {
		return 0
	}

	// 計算のためにコピーしてソート
	tmp := make([]float64, len(s.samples))
	copy(tmp, s.samples)
	sort.Float64s(tmp)

	index := int(p / 100.0 * float64(len(tmp)-1))
	return tmp[index]
}

func (s *LatencyStats) SampleCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.samples)
}

// ミドルウェア本体
func NewLatencyMiddleware(stats *LatencyStats) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		stats.Record(time.Since(start))
		return err
	}
}
