package rng

import "math/bits"

// Stream 是可确定性复现、可保存/恢复位置的独立随机数流。
// 底层为 PCG-XSH-RR (pcg32)，状态完整可序列化。
type Stream struct {
	seed     uint64
	state    uint64
	inc      uint64
	position uint64
}

const pcgMultiplier = 6364136223846793005

// Kind 标识一个仿真器内不同随机过程的子流用途。
type Kind uint64

const (
	KindArrival Kind = 1
	// KindServiceBase 为工作站服务时间子流基号（工作站 i 用 Base+i）。
	KindServiceBase Kind = 1000
)

// New 创建以 seed 为种子的随机数流。
func New(seed uint64) *Stream {
	s := &Stream{seed: seed, inc: 0xDA3E39CB94B95BDB}
	s.state = splitmix64(seed)
	s.next32()
	return s
}

// Uint64 返回下一个 64 位伪随机整数。
func (s *Stream) Uint64() uint64 {
	hi := uint64(s.next32())
	lo := uint64(s.next32())
	s.position++
	return hi<<32 | lo
}

// Float64 返回 [0,1) 上的下一个均匀伪随机数。
func (s *Stream) Float64() float64 {
	return float64(s.Uint64()>>11) * (1.0 / float64(uint64(1)<<53))
}

func (s *Stream) next32() uint32 {
	old := s.state
	s.state = old*pcgMultiplier + s.inc
	xorshifted := uint32(((old >> 18) ^ old) >> 27)
	rot := uint(old >> 59)
	return bits.RotateLeft32(xorshifted, -int(rot))
}

func (s *Stream) Position() uint64 { return s.position }

func splitmix64(z uint64) uint64 {
	z += 0x9E3779B97F4A7C15
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

type State struct {
	Seed     uint64
	PCGState uint64
	Inc      uint64
	Position uint64
}

func (s *Stream) StateOf() State {
	return State{Seed: s.seed, PCGState: s.state, Inc: s.inc, Position: s.position}
}

func Restore(state State) *Stream {
	return &Stream{seed: state.Seed, state: state.PCGState, inc: state.Inc, position: state.Position}
}

func ChildSeed(parent uint64, index int) uint64 {
	return splitmix64(parent ^ splitmix64(uint64(index)+0x9E3779B97F4A7C15))
}

func SubSeed(parent uint64, kind Kind) uint64 {
	return splitmix64(parent ^ splitmix64(uint64(kind)*0x9E3779B97F4A7C15))
}
