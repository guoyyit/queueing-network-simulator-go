package dist

import "math"

import "github.com/guoyyit/queueing-network-simulator-go/rng"

// Distribution 表示服务时间/到达间隔等随机量的分布。
// 分布本身无状态，随机性完全来自传入的 Stream。
type Distribution interface {
	Sample(r *rng.Stream) float64
}

// Exponential 为速率 Rate 的指数分布。
type Exponential struct {
	Rate float64
}

// Sample 用逆变换法采样：X = -ln(1-U)/Rate。
func (e Exponential) Sample(r *rng.Stream) float64 {
	u := 1.0 - r.Float64() // (0,1]，避免 ln(0)
	return -math.Log(u) / e.Rate
}

// Uniform 为 [Min, Max) 上的均匀分布。
type Uniform struct {
	Min float64
	Max float64
}

func (u Uniform) Sample(r *rng.Stream) float64 {
	return u.Min + (u.Max-u.Min)*r.Float64()
}

// Deterministic 为常数分布。
type Deterministic struct {
	Value float64
}

func (d Deterministic) Sample(r *rng.Stream) float64 { return d.Value }

// Erlang 为 k 阶 Erlang（形状 k、速率 Rate 的 Gamma 特例），
// 由 k 个独立指数变量之和采样得到。
type Erlang struct {
	K    int
	Rate float64
}

func (e Erlang) Sample(r *rng.Stream) float64 {
	sum := 0.0
	for i := 0; i < e.K; i++ {
		u := 1.0 - r.Float64()
		sum += -math.Log(u) / e.Rate
	}
	return sum
}

// Envelope 为分布提供带类型标签的 JSON 序列化（检查点使用）。
type Envelope struct {
	Type          string         `json:"type"`
	Exponential   *Exponential   `json:"exponential,omitempty"`
	Uniform       *Uniform       `json:"uniform,omitempty"`
	Deterministic *Deterministic `json:"deterministic,omitempty"`
	Erlang        *Erlang        `json:"erlang,omitempty"`
}

// Wrap 把已知分布包装为可 JSON 序列化的 Envelope。
func Wrap(d Distribution) Envelope {
	switch v := d.(type) {
	case Exponential:
		return Envelope{Type: "exponential", Exponential: &v}
	case *Exponential:
		return Envelope{Type: "exponential", Exponential: v}
	case Uniform:
		return Envelope{Type: "uniform", Uniform: &v}
	case *Uniform:
		return Envelope{Type: "uniform", Uniform: v}
	case Deterministic:
		return Envelope{Type: "deterministic", Deterministic: &v}
	case *Deterministic:
		return Envelope{Type: "deterministic", Deterministic: v}
	case Erlang:
		return Envelope{Type: "erlang", Erlang: &v}
	case *Erlang:
		return Envelope{Type: "erlang", Erlang: v}
	default:
		return Envelope{Type: "unknown"}
	}
}

// Unwrap 返回 Envelope 中的分布。
func (e Envelope) Unwrap() Distribution {
	switch e.Type {
	case "exponential":
		return *e.Exponential
	case "uniform":
		return *e.Uniform
	case "deterministic":
		return *e.Deterministic
	case "erlang":
		return *e.Erlang
	default:
		return nil
	}
}
