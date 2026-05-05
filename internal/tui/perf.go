package tui

import (
	"time"
)

const (
	perfBufferSize = 30
	sparkChars     = "▁▂▃▄▅▆▇█"
)

type PerfSample struct {
	CPU    float64
	Memory uint64
}

type PerfBuffer struct {
	samples     []PerfSample
	next        int
	size        int
	count       int
	prevCPUTime uint64
	prevTime    time.Time
}

func NewPerfBuffer() *PerfBuffer {
	return &PerfBuffer{
		samples: make([]PerfSample, perfBufferSize),
		size:    perfBufferSize,
	}
}

func (pb *PerfBuffer) AddSample(cpuTime uint64, memory uint64, now time.Time, vcpus uint) {
	if pb.prevCPUTime > 0 && vcpus > 0 {
		dt := now.Sub(pb.prevTime).Seconds()
		if dt > 0 {
			dcpu := float64(cpuTime-pb.prevCPUTime) / 1e9
			cpuPercent := dcpu / dt / float64(vcpus) * 100
			if cpuPercent < 0 {
				cpuPercent = 0
			}
			if cpuPercent > 100 {
				cpuPercent = 100 * float64(vcpus)
			}
			pb.samples[pb.next] = PerfSample{CPU: cpuPercent, Memory: memory}
			pb.next = (pb.next + 1) % pb.size
			if pb.count < pb.size {
				pb.count++
			}
		}
	}
	pb.prevCPUTime = cpuTime
	pb.prevTime = now
}

func (pb *PerfBuffer) CPUs() []float64 {
	if pb.count == 0 {
		return nil
	}
	start := (pb.next - pb.count + pb.size) % pb.size
	res := make([]float64, pb.count)
	for i := 0; i < pb.count; i++ {
		res[i] = pb.samples[(start+i)%pb.size].CPU
	}
	return res
}

func (pb *PerfBuffer) Memories() []float64 {
	if pb.count == 0 {
		return nil
	}
	start := (pb.next - pb.count + pb.size) % pb.size
	res := make([]float64, pb.count)
	for i := 0; i < pb.count; i++ {
		res[i] = float64(pb.samples[(start+i)%pb.size].Memory)
	}
	return res
}

func renderSparkline(values []float64, width int) string {
	if len(values) == 0 || width <= 0 {
		return ""
	}

	min, max := values[0], values[0]
	for _, v := range values {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}

	if max == min {
		max = min + 1
	}

	chars := []rune(sparkChars)
	result := make([]rune, width)

	for i := 0; i < width; i++ {
		idx := int(float64(i) * float64(len(values)) / float64(width))
		if idx >= len(values) {
			idx = len(values) - 1
		}

		normalized := (values[idx] - min) / (max - min)
		ci := int(normalized * float64(len(chars)-1))
		if ci < 0 {
			ci = 0
		}
		if ci >= len(chars) {
			ci = len(chars) - 1
		}
		result[i] = chars[ci]
	}
	return string(result)
}
